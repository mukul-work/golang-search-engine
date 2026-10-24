package db

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrator applies the embedded SQL migrations. It uses its own short-lived
// database/sql connection, separate from the Store's pool.
type Migrator struct {
	sqlDB    *sql.DB
	provider *goose.Provider
}

// MigrationStatus describes one known migration.
type MigrationStatus struct {
	Version int64
	Applied bool
}

func NewMigrator(dsn string) (*Migrator, error) {
	sqlDB, err := sql.Open("pgx", dsn) // does not connect yet
	if err != nil {
		return nil, err
	}
	fsys, err := fs.Sub(migrationsFS, "migrations") // goose wants the files at the root
	if err != nil {
		sqlDB.Close()
		return nil, err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys)
	if err != nil {
		sqlDB.Close()
		return nil, err
	}
	return &Migrator{sqlDB: sqlDB, provider: provider}, nil
}

func (m *Migrator) Close() error {
	return m.sqlDB.Close()
}

// Up applies every pending migration and returns the versions it applied.
func (m *Migrator) Up(ctx context.Context) ([]int64, error) {
	results, err := m.provider.Up(ctx)
	var versions []int64
	for _, r := range results {
		versions = append(versions, r.Source.Version)
	}
	return versions, err // on a failure, versions holds what succeeded before it
}

// Down rolls back the most recently applied migration.
func (m *Migrator) Down(ctx context.Context) (int64, error) {
	r, err := m.provider.Down(ctx)
	if err != nil {
		return 0, err
	}
	return r.Source.Version, nil
}

// Status lists every embedded migration and whether it has been applied.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MigrationStatus, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, MigrationStatus{
			Version: s.Source.Version,
			Applied: s.State == goose.StateApplied,
		})
	}
	return out, nil
}
