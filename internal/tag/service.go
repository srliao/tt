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
	ListWithCounts(ctx context.Context) ([]TagWithCount, error)
	GetByName(ctx context.Context, name string) (*Tag, error)
	Resolve(ctx context.Context, names []string, autoCreate bool) ([]int64, error)
	ResolveExisting(ctx context.Context, names []string) ([]int64, error)
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

// Create inserts a new tag with the given name. The name is normalized
// (trimmed + lowercased) before insertion; an empty or whitespace-only name is
// rejected, as is any name starting with the reserved `@` prefix (reserved
// for sentinel tokens like `@untagged` used by the tag_filter URL schema).
// To keep callers (especially Resolve) idempotent, Create first looks up the
// name and returns the existing tag if found instead of triggering a UNIQUE
// constraint error.
func (s *Impl) Create(ctx context.Context, name string) (Tag, error) {
	normalized := normalize(name)
	if err := validateName(normalized); err != nil {
		return Tag{}, err
	}

	if existing, err := s.GetByName(ctx, normalized); err != nil {
		return Tag{}, err
	} else if existing != nil {
		return *existing, nil
	}

	row, err := s.q.CreateTag(ctx, normalized)
	if err != nil {
		return Tag{}, fmt.Errorf("tag: create %q: %w", normalized, err)
	}
	return rowToTag(row), nil
}

// Rename updates the name of tag id. The new name is normalized (trimmed +
// lowercased) and rejected if empty.
func (s *Impl) Rename(ctx context.Context, id int64, name string) (Tag, error) {
	normalized := normalize(name)
	if err := validateName(normalized); err != nil {
		return Tag{}, err
	}
	row, err := s.q.RenameTag(ctx, sqlcgen.RenameTagParams{
		Name: normalized,
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

// ListWithCounts returns all tags in alphabetical order by name, each paired
// with the number of distinct tasks referencing the tag via task_tags. Tags
// with no tasks come back with Count == 0 (the LEFT JOIN preserves them).
func (s *Impl) ListWithCounts(ctx context.Context) ([]TagWithCount, error) {
	rows, err := s.q.ListTagsWithCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("tag: list with counts: %w", err)
	}
	out := make([]TagWithCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, TagWithCount{
			Tag: Tag{
				ID:        r.ID,
				Name:      r.Name,
				CreatedAt: parseSqliteTime(r.CreatedAt),
			},
			Count: r.Count,
		})
	}
	return out, nil
}

// GetByName returns the tag whose stored name matches the normalized form of
// the supplied name. Returns (nil, nil) when no row matches so callers can
// distinguish "missing" from "lookup failed".
func (s *Impl) GetByName(ctx context.Context, name string) (*Tag, error) {
	normalized := normalize(name)
	row, err := s.q.GetTagByName(ctx, normalized)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("tag: get by name %q: %w", normalized, err)
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
		n := normalize(raw)
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
		// Reserved sentinel names (e.g. "@untagged") can never name a real
		// tag — reject before touching the DB so callers see a uniform
		// validation error regardless of the autoCreate flag.
		if err := validateName(n); err != nil {
			return nil, err
		}
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

// ResolveExisting is the silently-tolerant sibling of Resolve: names that
// don't correspond to an existing tag are dropped rather than producing an
// error. The result preserves the order of first appearance and is
// deduplicated.
//
// Use this when the caller's semantics treat "tag not found" as a no-op
// (e.g. the bulk-tag remove path: removing a tag that doesn't exist on any
// task is functionally identical to a successful remove).
func (s *Impl) ResolveExisting(ctx context.Context, names []string) ([]int64, error) {
	seen := make(map[string]struct{}, len(names))
	unique := make([]string, 0, len(names))
	for _, raw := range names {
		n := normalize(raw)
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
	for _, n := range unique {
		existing, err := s.GetByName(ctx, n)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			continue
		}
		ids = append(ids, existing.ID)
	}
	return ids, nil
}

// normalize is the single canonical form for tag names: trimmed whitespace
// and lowercased. All entry points (Create, Rename, GetByName, Resolve) feed
// names through this so storage and lookups stay case-insensitive regardless
// of what the user typed.
func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// validateName enforces the basic shape rules for a normalized tag name.
// Empty names are rejected with the same "name is required" message used
// elsewhere; names starting with "@" are reserved for sentinel tokens
// surfaced through the tag_filter URL schema (e.g. "@untagged") and must
// never be persisted as a real tag.
func validateName(normalized string) error {
	if normalized == "" {
		return errors.New("tag: name is required")
	}
	if strings.HasPrefix(normalized, "@") {
		return fmt.Errorf("tag: invalid name %q: reserved prefix @", normalized)
	}
	return nil
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
