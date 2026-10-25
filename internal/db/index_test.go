package db

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// newMigratedStore returns a Store on a fresh, fully migrated, isolated schema.
func newMigratedStore(t *testing.T) *Store {
	t.Helper()
	dsn, _ := newTestSchema(t)
	ctx := context.Background()

	m := newTestMigrator(t, dsn)
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	store, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

// freqsFor returns word -> frequency for the page at url.
func freqsFor(t *testing.T, s *Store, url string) map[string]int {
	t.Helper()
	rows, err := s.pool.Query(context.Background(),
		`SELECT i.word, i.freq FROM inverted_index i JOIN pages p ON p.id = i.page_id WHERE p.url = $1`, url)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	got := map[string]int{}
	for rows.Next() {
		var word string
		var freq int
		if err := rows.Scan(&word, &freq); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[word] = freq
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return got
}

func countPages(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM pages`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestIndexPage_ReindexReplacesWords(t *testing.T) {
	store := newMigratedStore(t)
	ctx := context.Background()
	const url = "https://example.com/"

	if err := store.IndexPage(ctx, map[string]int{"alpha": 2, "beta": 1}, url, "t", "alpha alpha beta", 3); err != nil {
		t.Fatalf("first IndexPage: %v", err)
	}
	if err := store.IndexPage(ctx, map[string]int{"beta": 3, "gamma": 1}, url, "t", "beta beta beta gamma", 4); err != nil {
		t.Fatalf("second IndexPage: %v", err)
	}

	want := map[string]int{"beta": 3, "gamma": 1}
	if got := freqsFor(t, store, url); !reflect.DeepEqual(got, want) {
		t.Errorf("index = %v, want %v (alpha must be gone)", got, want)
	}
	if n := countPages(t, store); n != 1 {
		t.Errorf("pages = %d, want 1: a re-index must update the row, not add one", n)
	}
}

func TestIndexPage_ReindexLeavesOtherPagesAlone(t *testing.T) {
	store := newMigratedStore(t)
	ctx := context.Background()

	if err := store.IndexPage(ctx, map[string]int{"shared": 1, "onlya": 1}, "https://a.test/", "a", "x", 2); err != nil {
		t.Fatalf("IndexPage a: %v", err)
	}
	if err := store.IndexPage(ctx, map[string]int{"shared": 2}, "https://b.test/", "b", "x", 2); err != nil {
		t.Fatalf("IndexPage b: %v", err)
	}
	if err := store.IndexPage(ctx, map[string]int{"shared": 5}, "https://a.test/", "a", "x", 5); err != nil {
		t.Fatalf("re-IndexPage a: %v", err)
	}

	if got, want := freqsFor(t, store, "https://b.test/"), map[string]int{"shared": 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("page b = %v, want %v: re-indexing a must not touch b", got, want)
	}
	if got, want := freqsFor(t, store, "https://a.test/"), map[string]int{"shared": 5}; !reflect.DeepEqual(got, want) {
		t.Errorf("page a = %v, want %v", got, want)
	}
}

func TestIndexPage_RefreshesCrawledAt(t *testing.T) {
	store := newMigratedStore(t)
	ctx := context.Background()
	const url = "https://example.com/"

	read := func() time.Time {
		var ts time.Time
		if err := store.pool.QueryRow(ctx, `SELECT crawled_at FROM pages WHERE url = $1`, url).Scan(&ts); err != nil {
			t.Fatalf("reading crawled_at: %v", err)
		}
		return ts
	}

	if err := store.IndexPage(ctx, map[string]int{"a": 1}, url, "t", "a", 1); err != nil {
		t.Fatalf("first IndexPage: %v", err)
	}
	before := read()
	time.Sleep(20 * time.Millisecond)
	if err := store.IndexPage(ctx, map[string]int{"a": 1}, url, "t", "a", 1); err != nil {
		t.Fatalf("second IndexPage: %v", err)
	}
	if after := read(); !after.After(before) {
		t.Errorf("crawled_at did not advance: before %v, after %v", before, after)
	}
}

func TestIndexPage_PageWithNoWords(t *testing.T) {
	store := newMigratedStore(t)
	const url = "https://example.com/empty"

	if err := store.IndexPage(context.Background(), map[string]int{}, url, "", "", 0); err != nil {
		t.Fatalf("IndexPage with no words: %v", err)
	}
	if n := countPages(t, store); n != 1 {
		t.Errorf("pages = %d, want 1", n)
	}
	if got := freqsFor(t, store, url); len(got) != 0 {
		t.Errorf("index = %v, want empty", got)
	}
}
