package db

import (
	"context"
	"fmt"
	"sort"
	"testing"
)

func TestSearchPages_PagesAreStableAndTotalIsCorrect(t *testing.T) {
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
	t.Cleanup(store.Close) // registered after the schema cleanup, so it runs before it

	// 25 pages with the same word, frequency and length: every one gets the
	// exact same BM25 score, so only the p.id tiebreaker decides the order.
	const pageCount = 25
	for i := 0; i < pageCount; i++ {
		url := fmt.Sprintf("https://example.com/%02d", i) // zero-padded so text order = numeric order
		err := store.IndexPage(ctx, map[string]int{"gopher": 3}, url, "title", "gopher gopher gopher", 3)
		if err != nil {
			t.Fatalf("IndexPage: %v", err)
		}
	}

	var seen []string
	for offset := 0; offset < pageCount; offset += 10 {
		items, total, err := store.SearchPages(ctx, []string{"gopher"}, 10, offset)
		if err != nil {
			t.Fatalf("SearchPages(offset %d): %v", offset, err)
		}
		if total != pageCount {
			t.Errorf("offset %d: total = %d, want %d", offset, total, pageCount)
		}
		for _, it := range items {
			seen = append(seen, it.URL)
		}
	}

	if len(seen) != pageCount {
		t.Fatalf("collected %d results across pages, want %d", len(seen), pageCount)
	}
	if !sort.StringsAreSorted(seen) {
		t.Errorf("results are not in a stable order: %v", seen)
	}
	unique := map[string]struct{}{}
	for _, u := range seen {
		unique[u] = struct{}{}
	}
	if len(unique) != pageCount {
		t.Errorf("only %d distinct URLs across pages: a result was repeated or skipped", len(unique))
	}

	// Past the end no rows come back, so there is nowhere to read the total
	// from and it is 0. search.Service relies on this and looks the total up
	// separately.
	items, total, err := store.SearchPages(ctx, []string{"gopher"}, 10, 30)
	if err != nil {
		t.Fatalf("SearchPages past the end: %v", err)
	}
	if len(items) != 0 || total != 0 {
		t.Errorf("past the end: got %d items, total %d; want 0 and 0", len(items), total)
	}
}
