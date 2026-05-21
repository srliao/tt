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

// SetState transitions a task to st, managing completed_at / cancelled_at
// timestamps. Per spec §3, staged_order is intentionally untouched so a
// staged task remains visible in the focused batch after completion.
func (s *Impl) SetState(ctx context.Context, id int64, st State) (Task, error) {
	if !st.IsValid() {
		return Task{}, fmt.Errorf("task: invalid state %q", st)
	}

	now := time.Now().UTC().Format(sqliteTimeLayout)
	var completedAt, cancelledAt *string
	switch st {
	case StateDone:
		completedAt = &now
	case StateCancelled:
		cancelledAt = &now
	case StateNotDone:
		// both stay nil → clears any previous completed/cancelled timestamps
	}

	row, err := s.q.SetTaskState(ctx, sqlcgen.SetTaskStateParams{
		State:       string(st),
		CompletedAt: completedAt,
		CancelledAt: cancelledAt,
		ID:          id,
	})
	if err != nil {
		return Task{}, fmt.Errorf("task: set state: %w", err)
	}

	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return rowToTask(row, tags), nil
}

// Stage assigns the next ascending staged_order key to the task.
func (s *Impl) Stage(ctx context.Context, id int64) (Task, error) {
	maxS, err := s.maxStagedOrder(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("task: read max staged_order: %w", err)
	}
	newStaged := maxS + 1.0
	row, err := s.q.SetTaskStaged(ctx, sqlcgen.SetTaskStagedParams{
		StagedOrder: &newStaged,
		ID:          id,
	})
	if err != nil {
		return Task{}, fmt.Errorf("task: stage: %w", err)
	}
	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return rowToTask(row, tags), nil
}

// Unstage removes the task from the focused batch by setting staged_order
// back to NULL.
func (s *Impl) Unstage(ctx context.Context, id int64) (Task, error) {
	row, err := s.q.SetTaskStaged(ctx, sqlcgen.SetTaskStagedParams{
		StagedOrder: nil,
		ID:          id,
	})
	if err != nil {
		return Task{}, fmt.Errorf("task: unstage: %w", err)
	}
	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return rowToTask(row, tags), nil
}

// ClearStage empties the focused batch, regardless of each task's state.
func (s *Impl) ClearStage(ctx context.Context) error {
	if err := s.q.ClearStage(ctx); err != nil {
		return fmt.Errorf("task: clear stage: %w", err)
	}
	return nil
}

// ClearFinishedFromStage removes only the done / cancelled tasks from the
// focused batch, leaving still-actionable rows in place.
func (s *Impl) ClearFinishedFromStage(ctx context.Context) error {
	if err := s.q.ClearFinishedFromStage(ctx); err != nil {
		return fmt.Errorf("task: clear finished from stage: %w", err)
	}
	return nil
}

// ReorderMain moves task id between two visible neighbors in the main list.
// Pass beforeID=nil to drop at the top, afterID=nil to drop at the bottom.
// When the neighbors are too close together, the main list is rebalanced
// first and the neighbor keys re-read before computing the midpoint.
func (s *Impl) ReorderMain(ctx context.Context, id int64, beforeID, afterID *int64) (Task, error) {
	bp, ap, err := s.neighborPriorities(ctx, beforeID, afterID, false)
	if err != nil {
		return Task{}, err
	}
	if bp != nil && ap != nil && NeedsRebalance(*bp, *ap) {
		if err := s.RebalancePriority(ctx); err != nil {
			return Task{}, err
		}
		bp, ap, err = s.neighborPriorities(ctx, beforeID, afterID, false)
		if err != nil {
			return Task{}, err
		}
	}
	newKey := Midpoint(bp, ap)
	row, err := s.q.SetTaskPriority(ctx, sqlcgen.SetTaskPriorityParams{
		Priority: newKey,
		ID:       id,
	})
	if err != nil {
		return Task{}, fmt.Errorf("task: reorder main: %w", err)
	}
	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return rowToTask(row, tags), nil
}

// ReorderStage moves task id between two visible neighbors in the focused
// batch (staged_order). Mirrors ReorderMain semantics for the stage axis.
func (s *Impl) ReorderStage(ctx context.Context, id int64, beforeID, afterID *int64) (Task, error) {
	bp, ap, err := s.neighborPriorities(ctx, beforeID, afterID, true)
	if err != nil {
		return Task{}, err
	}
	if bp != nil && ap != nil && NeedsRebalance(*bp, *ap) {
		if err := s.RebalanceStage(ctx); err != nil {
			return Task{}, err
		}
		bp, ap, err = s.neighborPriorities(ctx, beforeID, afterID, true)
		if err != nil {
			return Task{}, err
		}
	}
	newKey := Midpoint(bp, ap)
	row, err := s.q.SetTaskStaged(ctx, sqlcgen.SetTaskStagedParams{
		StagedOrder: &newKey,
		ID:          id,
	})
	if err != nil {
		return Task{}, fmt.Errorf("task: reorder stage: %w", err)
	}
	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return rowToTask(row, tags), nil
}

// neighborPriorities looks up the priority (useStage=false) or staged_order
// (useStage=true) of each non-nil neighbor id. Returns an error if a
// referenced neighbor is unstaged but a stage neighbor was requested.
func (s *Impl) neighborPriorities(ctx context.Context, beforeID, afterID *int64, useStage bool) (*float64, *float64, error) {
	var bp, ap *float64
	if beforeID != nil {
		row, err := s.q.GetTask(ctx, *beforeID)
		if err != nil {
			return nil, nil, fmt.Errorf("task: reorder neighbor %d: %w", *beforeID, err)
		}
		k, err := neighborKey(row, useStage)
		if err != nil {
			return nil, nil, fmt.Errorf("task: before neighbor %d: %w", *beforeID, err)
		}
		bp = k
	}
	if afterID != nil {
		row, err := s.q.GetTask(ctx, *afterID)
		if err != nil {
			return nil, nil, fmt.Errorf("task: reorder neighbor %d: %w", *afterID, err)
		}
		k, err := neighborKey(row, useStage)
		if err != nil {
			return nil, nil, fmt.Errorf("task: after neighbor %d: %w", *afterID, err)
		}
		ap = k
	}
	return bp, ap, nil
}

// neighborKey returns the priority (useStage=false) or staged_order
// (useStage=true) of a row, erroring if the requested key is missing.
func neighborKey(row sqlcgen.Task, useStage bool) (*float64, error) {
	if useStage {
		if row.StagedOrder == nil {
			return nil, errors.New("neighbor is not staged")
		}
		v := *row.StagedOrder
		return &v, nil
	}
	v := row.Priority
	return &v, nil
}

// RebalancePriority is implemented in a later task; the stub keeps reorder
// callable until then. It is overwritten with the real implementation.
func (s *Impl) RebalancePriority(ctx context.Context) error { return nil }

// RebalanceStage is implemented in a later task; the stub keeps reorder
// callable until then.
func (s *Impl) RebalanceStage(ctx context.Context) error { return nil }

// Update replaces the mutable user-facing fields (title, notes, due_date)
// of a task. Title is required; due_date must be YYYY-MM-DD when present.
// Tag attachment is handled separately via SetTagsByID.
func (s *Impl) Update(ctx context.Context, id int64, in UpdateInput) (Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Task{}, errors.New("task: title is required")
	}
	due, err := normalizeDueDate(in.DueDate)
	if err != nil {
		return Task{}, err
	}

	row, err := s.q.UpdateTaskFields(ctx, sqlcgen.UpdateTaskFieldsParams{
		Title:   title,
		Notes:   in.Notes,
		DueDate: due,
		ID:      id,
	})
	if err != nil {
		return Task{}, fmt.Errorf("task: update: %w", err)
	}
	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return rowToTask(row, tags), nil
}

// Delete removes a task. CASCADE drops associated task_tags rows.
func (s *Impl) Delete(ctx context.Context, id int64) error {
	if err := s.q.DeleteTask(ctx, id); err != nil {
		return fmt.Errorf("task: delete %d: %w", id, err)
	}
	return nil
}

// Get loads a task by id along with its tag names.
func (s *Impl) Get(ctx context.Context, id int64) (Task, error) {
	row, err := s.q.GetTask(ctx, id)
	if err != nil {
		return Task{}, fmt.Errorf("task: get %d: %w", id, err)
	}
	tags, err := s.loadTags(ctx, id)
	if err != nil {
		return Task{}, err
	}
	return rowToTask(row, tags), nil
}

// loadTags returns the tag names associated with a task, sorted by name.
func (s *Impl) loadTags(ctx context.Context, taskID int64) ([]string, error) {
	rows, err := s.q.GetTaskTags(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task: load tags: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out, nil
}

// maxStagedOrder reads the current MAX(staged_order); -1.0 when no task is
// staged.
func (s *Impl) maxStagedOrder(ctx context.Context) (float64, error) {
	v, err := s.q.MaxStagedOrder(ctx)
	if err != nil {
		return 0, err
	}
	return coerceFloat(v), nil
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
