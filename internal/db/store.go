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
