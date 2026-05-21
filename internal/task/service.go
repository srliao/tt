package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/srliao/tt/internal/db"
	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
)

// sqliteTimeLayout matches the format goose's datetime('now') produces in
// SQLite (UTC, second precision). When parsing, we try RFC3339 as a fallback
// in case a caller persisted a higher-precision timestamp.
const sqliteTimeLayout = "2006-01-02 15:04:05"

// Service is the task-domain API. The interface enumerates every method
// implemented by Impl so callers can mock the service in tests.
type Service interface {
	// CRUD
	Create(ctx context.Context, in CreateInput) (Task, error)
	Get(ctx context.Context, id int64) (Task, error)
	Update(ctx context.Context, id int64, in UpdateInput) (Task, error)
	Delete(ctx context.Context, id int64) error

	// State transitions
	SetState(ctx context.Context, id int64, st State) (Task, error)

	// Staging
	Stage(ctx context.Context, id int64) (Task, error)
	Unstage(ctx context.Context, id int64) (Task, error)
	ClearStage(ctx context.Context) error
	ClearFinishedFromStage(ctx context.Context) error

	// Reorder / rebalance
	ReorderMain(ctx context.Context, id int64, beforeID, afterID *int64) (Task, error)
	ReorderStage(ctx context.Context, id int64, beforeID, afterID *int64) (Task, error)
	RebalancePriority(ctx context.Context) error
	RebalanceStage(ctx context.Context) error

	// Query
	List(ctx context.Context, f FilterSort) ([]Task, error)
	ByScript(ctx context.Context, scriptID int64, limit, offset int) ([]Task, error)
	LatestBySpawningScript(ctx context.Context, scriptID int64) (*Task, error)

	// Tags
	SetTagsByID(ctx context.Context, taskID int64, tagIDs []int64) error
}

// Impl is the concrete Service backed by a *db.Store. It is safe for
// concurrent use as the underlying *sql.DB is.
type Impl struct {
	store *db.Store
	q     *sqlcgen.Queries
}

// New constructs a Service bound to the supplied store.
func New(store *db.Store) *Impl {
	return &Impl{store: store, q: store.Queries()}
}

// Create inserts a new task with the next ascending priority key. Tag
// attachment is the caller's responsibility via SetTagsByID after they
// resolve tag names to ids through the tag service.
func (s *Impl) Create(ctx context.Context, in CreateInput) (Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Task{}, errors.New("task: title is required")
	}

	due, err := normalizeDueDate(in.DueDate)
	if err != nil {
		return Task{}, err
	}

	maxP, err := s.maxPriority(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("task: read max priority: %w", err)
	}
	newPriority := maxP + 1.0

	row, err := s.q.CreateTask(ctx, sqlcgen.CreateTaskParams{
		Title:             title,
		Notes:             in.Notes,
		DueDate:           due,
		Priority:          newPriority,
		StagedOrder:       nil,
		SpawnedByScriptID: in.SpawnedByScriptID,
	})
	if err != nil {
		return Task{}, fmt.Errorf("task: create: %w", err)
	}

	return rowToTask(row, nil), nil
}

// maxPriority reads the current max(priority); -1.0 when the table is empty.
func (s *Impl) maxPriority(ctx context.Context) (float64, error) {
	v, err := s.q.MaxPriority(ctx)
	if err != nil {
		return 0, err
	}
	return coerceFloat(v), nil
}

// normalizeDueDate validates a YYYY-MM-DD string, treating empty as "no
// due date". A nil pointer or empty string returns (nil, nil).
func normalizeDueDate(in *string) (*string, error) {
	if in == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*in)
	if trimmed == "" {
		return nil, nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return nil, fmt.Errorf("task: due_date must be YYYY-MM-DD: %w", err)
	}
	return &trimmed, nil
}

// rowToTask projects a sqlc-generated Task row into the domain type. Tags is
// supplied separately because it lives in a join table.
func rowToTask(r sqlcgen.Task, tags []string) Task {
	t := Task{
		ID:                r.ID,
		Title:             r.Title,
		Notes:             r.Notes,
		State:             State(r.State),
		DueDate:           r.DueDate,
		Priority:          r.Priority,
		StagedOrder:       r.StagedOrder,
		SpawnedByScriptID: r.SpawnedByScriptID,
		CreatedAt:         parseSqliteTime(r.CreatedAt),
		UpdatedAt:         parseSqliteTime(r.UpdatedAt),
		Tags:              tags,
	}
	if r.CompletedAt != nil {
		ts := parseSqliteTime(*r.CompletedAt)
		t.CompletedAt = &ts
	}
	if r.CancelledAt != nil {
		ts := parseSqliteTime(*r.CancelledAt)
		t.CancelledAt = &ts
	}
	return t
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

// coerceFloat converts the COALESCE(MAX(...), -1.0) result from sqlc back to a
// float64. modernc.org/sqlite scans the value into one of the numeric types
// depending on driver behavior, so we accept the common shapes defensively.
func coerceFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case nil:
		return -1.0
	}
	return -1.0
}
