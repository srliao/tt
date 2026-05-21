// Package db provides a SQLite-backed store with embedded goose migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/srliao/tt/internal/db/migrations"
)

// Store wraps a *sql.DB connection to the application's SQLite database.
// It is safe for concurrent use by multiple goroutines.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs all pending
// goose migrations. Pass ":memory:" to use an in-memory shared-cache database
// (useful for tests).
//
// The returned Store must be closed with Close when no longer needed.
func Open(ctx context.Context, path string) (*Store, error) {
	var dsn string
	if path == ":memory:" {
		// Shared cache so multiple connections see the same data;
		// foreign_keys must be enabled per-connection.
		dsn = "file::memory:?cache=shared&_pragma=foreign_keys(1)"
	} else {
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// In-memory databases live inside a single connection; using more than
	// one connection would expose separate empty databases.
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

	return &Store{db: sqlDB}, nil
}

// DB returns the underlying *sql.DB. Prefer using typed query methods where
// available; this accessor exists for callers that need raw SQL access.
func (s *Store) DB() *sql.DB {
	return s.db
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

func runMigrations(ctx context.Context, sqlDB *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
