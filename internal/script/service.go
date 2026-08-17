package script

import (
	"context"
	"encoding/json"
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

// Service is the script-domain API. Every method implemented by Impl appears
// here so callers can mock the service in tests.
type Service interface {
	// CRUD
	Create(ctx context.Context, in CreateInput) (Script, error)
	Update(ctx context.Context, id int64, in UpdateInput) (Script, error)
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (Script, error)
	List(ctx context.Context) ([]Script, error)

	// Scheduling
	DueAt(ctx context.Context, now time.Time, loc *time.Location) ([]Script, error)
	SetLastRunAt(ctx context.Context, id int64, t time.Time) error

	// User state
	ReadUserState(ctx context.Context, id int64) ([]byte, error)
	WriteUserState(ctx context.Context, id int64, blob []byte) error

	// Run lifecycle
	StartRun(ctx context.Context, scriptID int64, trigger Trigger) (Run, error)
	FinishRun(ctx context.Context, runID int64, status RunStatus, errMsg string, spawnedIDs []int64) error
	AppendLog(ctx context.Context, runID int64, level LogLevel, message string) error
	GetRun(ctx context.Context, runID int64) (Run, error)
	GetLogs(ctx context.Context, runID int64) ([]Log, error)
	ListRunsByScript(ctx context.Context, scriptID int64, limit, offset int) ([]Run, error)
	ListAllRuns(ctx context.Context, limit, offset int) ([]Run, error)
	CountRuns(ctx context.Context) (int64, error)
	PruneRuns(ctx context.Context, keep int64) error
	RecoverOrphanedRuns(ctx context.Context) error
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

// Create inserts a new script. Name is trimmed and required; Schedule is
// validated via MarshalConfig before storage so an invalid schedule fails fast
// rather than poisoning the row.
func (s *Impl) Create(ctx context.Context, in CreateInput) (Script, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Script{}, errors.New("script: name is required")
	}
	cfg, err := in.Schedule.MarshalConfig()
	if err != nil {
		return Script{}, err
	}
	row, err := s.q.CreateScript(ctx, sqlcgen.CreateScriptParams{
		Name:           name,
		Code:           in.Code,
		Enabled:        boolToInt(in.Enabled),
		ScheduleKind:   string(in.Schedule.Kind),
		ScheduleConfig: cfg,
	})
	if err != nil {
		return Script{}, fmt.Errorf("script: create: %w", err)
	}
	return rowToScript(row)
}

// Update replaces every user-facing field of a script. Name is trimmed and
// required; Schedule is re-marshalled to preserve the canonical JSON shape.
func (s *Impl) Update(ctx context.Context, id int64, in UpdateInput) (Script, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Script{}, errors.New("script: name is required")
	}
	cfg, err := in.Schedule.MarshalConfig()
	if err != nil {
		return Script{}, err
	}
	row, err := s.q.UpdateScript(ctx, sqlcgen.UpdateScriptParams{
		Name:           name,
		Code:           in.Code,
		Enabled:        boolToInt(in.Enabled),
		ScheduleKind:   string(in.Schedule.Kind),
		ScheduleConfig: cfg,
		ID:             id,
	})
	if err != nil {
		return Script{}, fmt.Errorf("script: update %d: %w", id, err)
	}
	return rowToScript(row)
}

// Delete removes a script. CASCADE removes its runs and logs.
func (s *Impl) Delete(ctx context.Context, id int64) error {
	if err := s.q.DeleteScript(ctx, id); err != nil {
		return fmt.Errorf("script: delete %d: %w", id, err)
	}
	return nil
}

// Get loads a script by id.
func (s *Impl) Get(ctx context.Context, id int64) (Script, error) {
	row, err := s.q.GetScript(ctx, id)
	if err != nil {
		return Script{}, fmt.Errorf("script: get %d: %w", id, err)
	}
	return rowToScript(row)
}

// List returns all scripts in (name, id) ascending order.
func (s *Impl) List(ctx context.Context) ([]Script, error) {
	rows, err := s.q.ListScripts(ctx)
	if err != nil {
		return nil, fmt.Errorf("script: list: %w", err)
	}
	out := make([]Script, 0, len(rows))
	for _, r := range rows {
		sc, err := rowToScript(r)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

// DueAt returns every enabled script whose schedule says "run" at now, with
// calendar days evaluated in loc (nil means UTC). The scheduler can iterate
// this list directly to launch runs.
func (s *Impl) DueAt(ctx context.Context, now time.Time, loc *time.Location) ([]Script, error) {
	rows, err := s.q.ListEnabledScripts(ctx)
	if err != nil {
		return nil, fmt.Errorf("script: list enabled: %w", err)
	}
	out := make([]Script, 0, len(rows))
	for _, r := range rows {
		sc, err := rowToScript(r)
		if err != nil {
			return nil, err
		}
		if sc.Schedule.Matches(now, sc.LastRunAt, loc) {
			out = append(out, sc)
		}
	}
	return out, nil
}

// SetLastRunAt stamps the supplied time on the script row using SQLite's
// canonical "YYYY-MM-DD HH:MM:SS" UTC layout so future reads round-trip
// cleanly.
func (s *Impl) SetLastRunAt(ctx context.Context, id int64, t time.Time) error {
	ts := t.UTC().Format(sqliteTimeLayout)
	if err := s.q.SetScriptLastRunAt(ctx, sqlcgen.SetScriptLastRunAtParams{
		LastRunAt: &ts,
		ID:        id,
	}); err != nil {
		return fmt.Errorf("script: set last_run_at %d: %w", id, err)
	}
	return nil
}

// ReadUserState returns the raw JSON blob stored on the script row.
func (s *Impl) ReadUserState(ctx context.Context, id int64) ([]byte, error) {
	v, err := s.q.GetScriptUserState(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("script: read user_state %d: %w", id, err)
	}
	return []byte(v), nil
}

// WriteUserState persists the JSON blob. An empty blob is stored as "{}" so
// the column never violates the (implicit) JSON-shape expectation downstream.
func (s *Impl) WriteUserState(ctx context.Context, id int64, blob []byte) error {
	v := string(blob)
	if v == "" {
		v = "{}"
	}
	if err := s.q.SetScriptUserState(ctx, sqlcgen.SetScriptUserStateParams{
		UserState: v,
		ID:        id,
	}); err != nil {
		return fmt.Errorf("script: write user_state %d: %w", id, err)
	}
	return nil
}

// StartRun inserts a new script_runs row in the "running" state.
func (s *Impl) StartRun(ctx context.Context, scriptID int64, trigger Trigger) (Run, error) {
	row, err := s.q.CreateScriptRun(ctx, sqlcgen.CreateScriptRunParams{
		ScriptID: scriptID,
		Trigger:  string(trigger),
	})
	if err != nil {
		return Run{}, fmt.Errorf("script: start run %d: %w", scriptID, err)
	}
	return rowToRun(row)
}

// FinishRun stamps the terminal state on a run. errMsg=="" omits the error
// column (stored as NULL via a nil *string). spawnedIDs is JSON-encoded;
// nil/empty becomes "[]" to satisfy the NOT NULL DEFAULT '[]' column.
func (s *Impl) FinishRun(ctx context.Context, runID int64, status RunStatus, errMsg string, spawnedIDs []int64) error {
	if spawnedIDs == nil {
		spawnedIDs = []int64{}
	}
	b, err := json.Marshal(spawnedIDs)
	if err != nil {
		return fmt.Errorf("script: marshal spawned ids: %w", err)
	}
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	if err := s.q.FinishScriptRun(ctx, sqlcgen.FinishScriptRunParams{
		Status:         string(status),
		ErrorMessage:   errPtr,
		SpawnedTaskIds: string(b),
		ID:             runID,
	}); err != nil {
		return fmt.Errorf("script: finish run %d: %w", runID, err)
	}
	return nil
}

// AppendLog appends a single log line to a run.
func (s *Impl) AppendLog(ctx context.Context, runID int64, level LogLevel, message string) error {
	if err := s.q.AppendScriptLog(ctx, sqlcgen.AppendScriptLogParams{
		ScriptRunID: runID,
		Level:       string(level),
		Message:     message,
	}); err != nil {
		return fmt.Errorf("script: append log: %w", err)
	}
	return nil
}

// GetRun loads a single run by id.
func (s *Impl) GetRun(ctx context.Context, runID int64) (Run, error) {
	row, err := s.q.GetScriptRun(ctx, runID)
	if err != nil {
		return Run{}, fmt.Errorf("script: get run %d: %w", runID, err)
	}
	return rowToRun(row)
}

// GetLogs returns every log line of a run in ascending order.
func (s *Impl) GetLogs(ctx context.Context, runID int64) ([]Log, error) {
	rows, err := s.q.ListScriptLogsByRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("script: list logs %d: %w", runID, err)
	}
	out := make([]Log, 0, len(rows))
	for _, r := range rows {
		out = append(out, Log{
			ID:       r.ID,
			RunID:    r.ScriptRunID,
			Level:    LogLevel(r.Level),
			Message:  r.Message,
			LoggedAt: parseSqliteTime(r.LoggedAt),
		})
	}
	return out, nil
}

// ListRunsByScript returns runs for a single script, newest first.
func (s *Impl) ListRunsByScript(ctx context.Context, scriptID int64, limit, offset int) ([]Run, error) {
	if limit <= 0 {
		limit = -1
	}
	rows, err := s.q.ListScriptRunsByScript(ctx, sqlcgen.ListScriptRunsByScriptParams{
		ScriptID: scriptID,
		Limit:    int64(limit),
		Offset:   int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("script: list runs by script %d: %w", scriptID, err)
	}
	return runsToDomain(rows)
}

// ListAllRuns returns runs across all scripts, newest first.
func (s *Impl) ListAllRuns(ctx context.Context, limit, offset int) ([]Run, error) {
	if limit <= 0 {
		limit = -1
	}
	rows, err := s.q.ListAllScriptRuns(ctx, sqlcgen.ListAllScriptRunsParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("script: list all runs: %w", err)
	}
	return runsToDomain(rows)
}

// CountRuns returns the total number of script_runs rows. Used to drive
// retention pruning.
func (s *Impl) CountRuns(ctx context.Context) (int64, error) {
	n, err := s.q.CountScriptRuns(ctx)
	if err != nil {
		return 0, fmt.Errorf("script: count runs: %w", err)
	}
	return n, nil
}

// PruneRuns deletes the oldest runs until at most keep remain. Logs cascade
// out automatically because script_logs.script_run_id has ON DELETE CASCADE.
func (s *Impl) PruneRuns(ctx context.Context, keep int64) error {
	count, err := s.CountRuns(ctx)
	if err != nil {
		return err
	}
	if keep >= count {
		return nil
	}
	if err := s.q.DeleteOldestScriptRuns(ctx, count-keep); err != nil {
		return fmt.Errorf("script: prune runs: %w", err)
	}
	return nil
}

// RecoverOrphanedRuns marks every still-"running" row as errored. Intended
// for binary startup: any run that was in flight when the previous process
// died is otherwise stuck visible as running forever.
func (s *Impl) RecoverOrphanedRuns(ctx context.Context) error {
	if err := s.q.MarkOrphanedRunsAsError(ctx); err != nil {
		return fmt.Errorf("script: recover orphaned runs: %w", err)
	}
	return nil
}

// rowToScript projects a sqlc-generated Script row into the domain type,
// parsing the schedule_config JSON and any persisted timestamps.
func rowToScript(r sqlcgen.Script) (Script, error) {
	sch, err := ParseSchedule(r.ScheduleKind, r.ScheduleConfig)
	if err != nil {
		return Script{}, fmt.Errorf("script: row %d: %w", r.ID, err)
	}
	sc := Script{
		ID:        r.ID,
		Name:      r.Name,
		Code:      r.Code,
		Enabled:   r.Enabled == 1,
		Schedule:  sch,
		CreatedAt: parseSqliteTime(r.CreatedAt),
		UpdatedAt: parseSqliteTime(r.UpdatedAt),
	}
	if r.LastRunAt != nil {
		ts := parseSqliteTime(*r.LastRunAt)
		sc.LastRunAt = &ts
	}
	return sc, nil
}

// rowToRun projects a sqlc-generated ScriptRun row into the domain type,
// decoding spawned_task_ids from JSON.
func rowToRun(r sqlcgen.ScriptRun) (Run, error) {
	ids := []int64{}
	if strings.TrimSpace(r.SpawnedTaskIds) != "" {
		if err := json.Unmarshal([]byte(r.SpawnedTaskIds), &ids); err != nil {
			return Run{}, fmt.Errorf("script: run %d: parse spawned_task_ids: %w", r.ID, err)
		}
		if ids == nil {
			ids = []int64{}
		}
	}
	run := Run{
		ID:             r.ID,
		ScriptID:       r.ScriptID,
		StartedAt:      parseSqliteTime(r.StartedAt),
		Status:         RunStatus(r.Status),
		SpawnedTaskIDs: ids,
		Trigger:        Trigger(r.Trigger),
	}
	if r.FinishedAt != nil {
		ts := parseSqliteTime(*r.FinishedAt)
		run.FinishedAt = &ts
	}
	if r.ErrorMessage != nil {
		run.ErrorMessage = *r.ErrorMessage
	}
	return run, nil
}

// runsToDomain converts a slice of sqlc rows. Pulled out so the two list
// helpers don't duplicate the loop body.
func runsToDomain(rows []sqlcgen.ScriptRun) ([]Run, error) {
	out := make([]Run, 0, len(rows))
	for _, r := range rows {
		run, err := rowToRun(r)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

// boolToInt encodes a Go bool as the 0/1 form SQLite stores enabled with.
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// parseSqliteTime accepts both SQLite's datetime('now') output and RFC3339
// to be tolerant of timestamps produced by other code paths. Duplicated
// across task/tag/script to keep each domain package self-contained.
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
