// Package db provides a SQLite-backed store with embedded goose migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/srliao/tt/internal/db/migrations"
	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
)

// Store wraps a *sql.DB connection to the application's SQLite database.
// It is safe for concurrent use by multiple goroutines.
type Store struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// Open opens (or creates) the SQLite database at path and runs all pending
// goose migrations. Pass ":memory:" to use an in-memory database (useful for
// tests); SetMaxOpenConns(1) pins the in-memory pool to a single connection
// so every caller sees the same database.
//
// The returned Store must be closed with Close when no longer needed.
func Open(ctx context.Context, path string) (*Store, error) {
	var dsn string
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	} else {
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if path == ":memory:" {
		sqlDB.SetMaxOpenConns(1)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := runMigrations(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{
		db:      sqlDB,
		queries: sqlcgen.New(sqlDB),
	}, nil
}

// DB returns the underlying *sql.DB. Prefer using typed query methods where
// available; this accessor exists for callers that need raw SQL access.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Queries returns the sqlc-generated typed query handle bound to this store's
// connection. Use Queries.WithTx for queries that must run inside a
// transaction.
func (s *Store) Queries() *sqlcgen.Queries {
	return s.queries
}

// VacuumInto writes a transactionally consistent copy of the database,
// including the goose version table, to path. path must not already exist.
func (s *Store) VacuumInto(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}

// VersionMismatchError reports a snapshot whose goose version differs from
// the latest migration this binary embeds.
type VersionMismatchError struct {
	Snapshot int64
	Latest   int64
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("snapshot schema version %d differs from this binary's %d", e.Snapshot, e.Latest)
}

// OpenSnapshot loads a database file image (as written by VacuumInto) into
// in-memory SQLite. Migrations are not run: the image must already be at
// this binary's latest migration, else a *VersionMismatchError is returned.
func OpenSnapshot(ctx context.Context, image []byte) (*Store, error) {
	if len(image) == 0 {
		return nil, fmt.Errorf("open snapshot: empty image")
	}
	sqlDB, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	store, err := loadSnapshot(ctx, sqlDB, image)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return store, nil
}

func loadSnapshot(ctx context.Context, sqlDB *sql.DB, image []byte) (*Store, error) {
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open snapshot: %w", err)
	}
	err = conn.Raw(func(dc any) error {
		d, ok := dc.(interface{ Deserialize([]byte) error })
		if !ok {
			return fmt.Errorf("sqlite driver cannot deserialize")
		}
		return d.Deserialize(image)
	})
	_ = conn.Close()
	if err != nil {
		return nil, fmt.Errorf("open snapshot: deserialize: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.FS)
	if err != nil {
		return nil, fmt.Errorf("new goose provider: %w", err)
	}
	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("open snapshot: read schema version: %w", err)
	}
	if latest := latestVersion(provider); current != latest {
		return nil, &VersionMismatchError{Snapshot: current, Latest: latest}
	}
	return &Store{db: sqlDB, queries: sqlcgen.New(sqlDB)}, nil
}

// SchemaVersion is the goose version of the latest migration this binary
// embeds: the schema every database it opens is migrated to, and the one a
// snapshot must match.
func SchemaVersion() (int64, error) {
	// sql.Open does not connect; the provider only reads the embedded files.
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return 0, fmt.Errorf("open sqlite: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.FS)
	if err != nil {
		return 0, fmt.Errorf("new goose provider: %w", err)
	}
	return latestVersion(provider), nil
}

func latestVersion(provider *goose.Provider) int64 {
	sources := provider.ListSources()
	if len(sources) == 0 {
		return 0
	}
	return sources[len(sources)-1].Version
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close sqlite: %w", err)
	}
	return nil
}

// runMigrations uses goose.NewProvider (instance-scoped) instead of the
// global SetBaseFS/SetDialect pair, so concurrent Open calls from parallel
// test packages don't race on goose's package-level state.
func runMigrations(ctx context.Context, sqlDB *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("new goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
