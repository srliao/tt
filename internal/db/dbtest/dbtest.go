// Package dbtest provides test helpers for spinning up ephemeral SQLite
// databases backed by the production migrations.
package dbtest

import (
	"context"
	"testing"

	"github.com/srliao/tt/internal/db"
)

// New opens a fresh in-memory SQLite database via db.Open(":memory:"),
// applies all migrations, and registers cleanup on the test. It is intended
// for unit tests that need a working *db.Store without touching disk.
func New(t *testing.T) *db.Store {
	t.Helper()

	store, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("dbtest.New: open in-memory store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
