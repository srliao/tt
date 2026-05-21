package tag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/srliao/tt/internal/db"
	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
)

// sqliteTimeLayout matches the format SQLite's datetime('now') produces (UTC,
// second precision). RFC3339 is tried as a fallback in case a caller persisted
// a higher-precision timestamp.
const sqliteTimeLayout = "2006-01-02 15:04:05"

// Service is the tag-domain API. The interface enumerates every method
// implemented by Impl so callers can mock the service in tests.
type Service interface {
	Create(ctx context.Context, name string) (Tag, error)
	Rename(ctx context.Context, id int64, name string) (Tag, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]Tag, error)
	GetByName(ctx context.Context, name string) (*Tag, error)
	Resolve(ctx context.Context, names []string, autoCreate bool) ([]int64, error)
}

// Impl is the concrete Service backed by a *db.Store.
type Impl struct {
	store *db.Store
	q     *sqlcgen.Queries
}

// New constructs a Service bound to the supplied store.
func New(store *db.Store) *Impl {
	return &Impl{store: store, q: store.Queries()}
}

// Create inserts a new tag with the given name. The name is trimmed before
// insertion; an empty or whitespace-only name is rejected. To keep callers
// (especially Resolve) idempotent, Create first looks up the name and returns
// the existing tag if found instead of triggering a UNIQUE constraint error.
func (s *Impl) Create(ctx context.Context, name string) (Tag, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Tag{}, errors.New("tag: name is required")
	}

	if existing, err := s.GetByName(ctx, trimmed); err != nil {
		return Tag{}, err
	} else if existing != nil {
		return *existing, nil
	}

	row, err := s.q.CreateTag(ctx, trimmed)
	if err != nil {
		return Tag{}, fmt.Errorf("tag: create %q: %w", trimmed, err)
	}
	return rowToTag(row), nil
}

// Rename updates the name of tag id. The new name is trimmed and rejected if
// empty.
func (s *Impl) Rename(ctx context.Context, id int64, name string) (Tag, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Tag{}, errors.New("tag: name is required")
	}
	row, err := s.q.RenameTag(ctx, sqlcgen.RenameTagParams{
		Name: trimmed,
		ID:   id,
	})
	if err != nil {
		return Tag{}, fmt.Errorf("tag: rename %d: %w", id, err)
	}
	return rowToTag(row), nil
}

// Delete removes a tag. CASCADE drops associated task_tags rows.
func (s *Impl) Delete(ctx context.Context, id int64) error {
	if err := s.q.DeleteTag(ctx, id); err != nil {
		return fmt.Errorf("tag: delete %d: %w", id, err)
	}
	return nil
}

// List returns all tags in alphabetical order by name.
func (s *Impl) List(ctx context.Context) ([]Tag, error) {
	rows, err := s.q.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("tag: list: %w", err)
	}
	out := make([]Tag, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToTag(r))
	}
	return out, nil
}

// GetByName returns the tag with the supplied (untrimmed) name. Returns
// (nil, nil) when no row matches so callers can distinguish "missing" from
// "lookup failed".
func (s *Impl) GetByName(ctx context.Context, name string) (*Tag, error) {
	row, err := s.q.GetTagByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("tag: get by name %q: %w", name, err)
	}
	t := rowToTag(row)
	return &t, nil
}

// Resolve converts a list of tag names into the corresponding tag ids,
// trimming each name, dropping empties, and deduplicating preserving the
// order of first appearance. When autoCreate is true, missing names are
// inserted on the fly; otherwise the missing names are accumulated and
// returned as a single error so the caller can report them all at once.
func (s *Impl) Resolve(ctx context.Context, names []string, autoCreate bool) ([]int64, error) {
	seen := make(map[string]struct{}, len(names))
	unique := make([]string, 0, len(names))
	for _, raw := range names {
		n := strings.TrimSpace(raw)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		unique = append(unique, n)
	}

	ids := make([]int64, 0, len(unique))
	var missing []string
	for _, n := range unique {
		existing, err := s.GetByName(ctx, n)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			ids = append(ids, existing.ID)
			continue
		}
		if !autoCreate {
			missing = append(missing, n)
			continue
		}
		row, err := s.q.CreateTag(ctx, n)
		if err != nil {
			return nil, fmt.Errorf("tag: resolve create %q: %w", n, err)
		}
		ids = append(ids, row.ID)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("tag: unknown tags: %s", strings.Join(missing, ", "))
	}
	return ids, nil
}

// rowToTag projects a sqlc-generated Tag row into the domain type.
func rowToTag(r sqlcgen.Tag) Tag {
	return Tag{
		ID:        r.ID,
		Name:      r.Name,
		CreatedAt: parseSqliteTime(r.CreatedAt),
	}
}

// parseSqliteTime accepts both SQLite's datetime('now') output and RFC3339
// to be tolerant of timestamps produced by other code paths.
func parseSqliteTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(sqliteTimeLayout, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// Compile-time assertion that Impl satisfies the Service interface.
var _ Service = (*Impl)(nil)
