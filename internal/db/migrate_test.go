package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// newTestSchema creates an isolated schema and returns a DSN whose search_path
// points at it, plus the schema's name. It skips the test if TEST_DB_URL is unset.
func newTestSchema(t *testing.T) (dsn, schema string) {
	t.Helper()
	base := os.Getenv("TEST_DB_URL")
	if base == "" {
		t.Skip("TEST_DB_URL not set; skipping database test")
	}

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	schema = fmt.Sprintf("migtest_%d", time.Now().UnixNano()) // digits only: safe as an identifier
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { // registered first, so it runs last (after the test's own connections close)
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("TEST_DB_URL must be a postgres:// URL: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), schema
}

func openSQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	d, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func newTestMigrator(t *testing.T, dsn string) *Migrator {
	t.Helper()
	m, err := NewMigrator(dsn)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func tableExists(t *testing.T, d *sql.DB, schema, table string) bool {
	t.Helper()
	var exists bool
	err := d.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2)`,
		schema, table,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("checking for table %s: %v", table, err)
	}
	return exists
}

// The database as it looked before migrations existed (the old schema.sql).
var preMigrationSchema = []string{
	`CREATE TABLE pages (
		id SERIAL PRIMARY KEY,
		url TEXT UNIQUE NOT NULL,
		title TEXT,
		content TEXT,
		total_words INT,
		crawled_at TIMESTAMP DEFAULT NOW()
	)`,
	`CREATE TABLE inverted_index (
		word TEXT NOT NULL,
		page_id INT REFERENCES pages(id) ON DELETE CASCADE,
		freq INT NOT NULL,
		PRIMARY KEY (word, page_id)
	)`,
	`CREATE INDEX idx_inverted_index_word ON inverted_index(word)`,
}

// Needs no database: guards against a migration file without a Down section.
func TestMigrationFiles_HaveUpAndDown(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no migrations embedded")
	}
	for _, e := range entries {
		b, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, marker := range []string{"-- +goose Up", "-- +goose Down"} {
			if !strings.Contains(string(b), marker) {
				t.Errorf("%s is missing %q", e.Name(), marker)
			}
		}
	}
}

func TestMigrations_UpIsIdempotent(t *testing.T) {
	dsn, schema := newTestSchema(t)
	check := openSQL(t, dsn)
	m := newTestMigrator(t, dsn)
	ctx := context.Background()

	applied, err := m.Up(ctx)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("Up applied nothing on a fresh schema")
	}
	for _, table := range []string{"pages", "inverted_index"} {
		if !tableExists(t, check, schema, table) {
			t.Errorf("table %s was not created", table)
		}
	}

	again, err := m.Up(ctx)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second Up applied %v, want nothing", again)
	}
}

func TestMigrations_DownRemovesEverything(t *testing.T) {
	dsn, schema := newTestSchema(t)
	check := openSQL(t, dsn)
	m := newTestMigrator(t, dsn)
	ctx := context.Background()

	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	statuses, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	for range statuses { // roll back every migration, however many exist
		if _, err := m.Down(ctx); err != nil {
			t.Fatalf("Down: %v", err)
		}
	}
	for _, table := range []string{"pages", "inverted_index"} {
		if tableExists(t, check, schema, table) {
			t.Errorf("table %s still exists after rolling everything back", table)
		}
	}
}

// The real-world case: a database that already has the tables from schema.sql.
func TestMigrations_BaselineAdoptsExistingSchema(t *testing.T) {
	dsn, _ := newTestSchema(t)
	check := openSQL(t, dsn)
	for _, stmt := range preMigrationSchema {
		if _, err := check.Exec(stmt); err != nil {
			t.Fatalf("setting up the old schema: %v", err)
		}
	}
	if _, err := check.Exec(
		`INSERT INTO pages (url, title, content, total_words) VALUES ('https://example.com/', 't', 'c', 1)`,
	); err != nil {
		t.Fatalf("inserting a row: %v", err)
	}

	m := newTestMigrator(t, dsn)
	if _, err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up on a database that already has the tables failed: %v", err)
	}

	var n int
	if err := check.QueryRow(`SELECT count(*) FROM pages`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("pages has %d rows after Up, want the 1 we inserted", n)
	}
}

func TestMigrations_IndexesAfterUp(t *testing.T) {
	dsn, schema := newTestSchema(t)
	check := openSQL(t, dsn)
	m := newTestMigrator(t, dsn)
	if _, err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}

	rows, err := check.Query(
		`SELECT indexname FROM pg_indexes WHERE schemaname = $1 AND tablename = 'inverted_index'`, schema)
	if err != nil {
		t.Fatalf("listing indexes: %v", err)
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating indexes: %v", err)
	}

	for _, want := range []string{"inverted_index_pkey", "idx_inverted_index_page_id"} {
		if !have[want] {
			t.Errorf("index %s is missing; have %v", want, have)
		}
	}
	if have["idx_inverted_index_word"] {
		t.Error("idx_inverted_index_word should have been dropped: the primary key already covers word lookups")
	}
}
