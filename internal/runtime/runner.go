// Package runtime executes user scripts inside an isolated goja runtime.
//
// Each invocation of Runner.Run is a self-contained "run": a fresh
// *goja.Runtime is constructed, the ctx API is installed (see ctx.go), the
// script code is executed under a hard 5-second timeout, and the resulting
// effects are applied per spec §5:
//
//   - Logs (ctx.log.*, console.*) write through to script_logs immediately.
//     They survive script errors and timeouts.
//   - ctx.state.set / delete are buffered and flush to scripts.user_state
//     only when the run finishes ok.
//   - ctx.queueTask is buffered and flushes — via task.Service +
//     tag.Service — only when the run finishes ok.
//   - scripts.last_run_at is stamped regardless of outcome.
//
// Run ownership: the caller (HTTP handler / scheduler) calls
// script.Service.StartRun before invoking Runner.Run, then passes the
// returned runID in. The runtime never creates run rows.
//
// Consumer interface: downstream callers (scheduler, HTTP server) declare a
// narrow interface that *Runner satisfies structurally:
//
//	type Runner interface {
//	    Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
//	}
//
// The exampleRunner type at the bottom of this file pins the contract at
// compile time.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/dop251/goja"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

// defaultTimeout is the per-run wall-clock budget enforced via
// goja.Runtime.Interrupt. Spec §5 fixes this at 5s; tests can override via
// WithTimeout.
const defaultTimeout = 5 * time.Second

// runRetention is the maximum number of script_runs rows kept around. Older
// runs are pruned at the end of every Run invocation so the table never
// drifts beyond this size.
const runRetention = 500

// Runner executes user scripts. It is safe to share a single Runner across
// goroutines because each Run constructs its own isolated goja runtime.
type Runner struct {
	tasks   task.Service
	tags    tag.Service
	scripts script.Service
	logger  *slog.Logger
	now     func() time.Time
	timeout time.Duration
}

// Option mutates a *Runner during construction.
type Option func(*Runner)

// WithTimeout overrides the default 5s per-run timeout. Intended for tests
// that need fast feedback on the interrupt path.
func WithTimeout(d time.Duration) Option {
	return func(r *Runner) {
		if d > 0 {
			r.timeout = d
		}
	}
}

// WithClock overrides the function used to obtain the current time. Defaults
// to time.Now; tests use this to inject a deterministic clock so date
// helpers produce stable output.
func WithClock(now func() time.Time) Option {
	return func(r *Runner) {
		if now != nil {
			r.now = now
		}
	}
}

// New constructs a Runner backed by the supplied services. The logger is
// used only for runtime-internal diagnostics (panic recovery, persistence
// failures); user script output flows through ctx.log instead.
func New(
	tasks task.Service,
	tags tag.Service,
	scripts script.Service,
	logger *slog.Logger,
	opts ...Option,
) *Runner {
	r := &Runner{
		tasks:   tasks,
		tags:    tags,
		scripts: scripts,
		logger:  logger,
		now:     time.Now,
		timeout: defaultTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run executes the script identified by scriptID against the pre-created
// runID. The caller is responsible for obtaining runID via
// script.Service.StartRun before invoking this method.
//
// Run is best-effort: every persistence failure within it is logged but the
// method swallows the error and continues, because half-finished cleanup is
// worse than a stale row that future code can detect and reconcile. The
// returned error is non-nil only when the run could not start at all (e.g.
// the script row was deleted between StartRun and Run).
func (r *Runner) Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error {
	sc, err := r.scripts.Get(ctx, scriptID)
	if err != nil {
		return fmt.Errorf("runtime: load script %d: %w", scriptID, err)
	}
	if !sc.Enabled {
		// A disabled script that nonetheless has a running row is an
		// inconsistency that should be visible as an error rather than a
		// silent no-op.
		finishErr := r.scripts.FinishRun(ctx, runID, script.RunStatusError, "script is disabled", nil)
		if finishErr != nil {
			r.logger.Error("finish disabled run", "run_id", runID, "err", finishErr)
		}
		return nil
	}

	now := r.now()

	// last_run_at + prune always run, regardless of how the run terminated.
	defer func() {
		if err := r.scripts.SetLastRunAt(ctx, scriptID, now); err != nil {
			r.logger.Error("set last_run_at", "script_id", scriptID, "err", err)
		}
		if err := r.scripts.PruneRuns(ctx, runRetention); err != nil {
			r.logger.Error("prune runs", "err", err)
		}
	}()

	// Load existing state. An empty / corrupt blob is treated as "no
	// state" so a malformed write from a prior run can't brick the script.
	stateBuf := r.loadStateBuffer(ctx, scriptID)

	queue := newTaskQueue()

	// Resolve lastSpawn ahead of execution so the ctx object is fully
	// populated before the JS sees it. A nil pointer becomes JS null.
	lastTask, err := r.tasks.LatestBySpawningScript(ctx, scriptID)
	if err != nil {
		r.logger.Error("load last spawn", "script_id", scriptID, "err", err)
		lastTask = nil
	}

	rt := goja.New()
	logFn := logBindings(r.scripts, runID)

	if err := installCtx(ctxDeps{
		rt:       rt,
		now:      now,
		sc:       sc,
		trigger:  trigger,
		state:    stateBuf,
		queue:    queue,
		logFn:    logFn,
		runCtx:   ctx,
		lastTask: lastTask,
	}); err != nil {
		r.failRun(ctx, runID, err.Error())
		return nil
	}

	status, errMsg := r.execute(rt, sc.Code)

	switch status {
	case script.RunStatusOK:
		spawnedIDs := r.flushQueue(ctx, scriptID, queue)
		r.persistState(ctx, scriptID, stateBuf)
		if err := r.scripts.FinishRun(ctx, runID, script.RunStatusOK, "", spawnedIDs); err != nil {
			r.logger.Error("finish ok run", "run_id", runID, "err", err)
		}
	case script.RunStatusTimeout:
		if err := r.scripts.FinishRun(ctx, runID, script.RunStatusTimeout, errMsg, nil); err != nil {
			r.logger.Error("finish timeout run", "run_id", runID, "err", err)
		}
	default:
		if err := r.scripts.FinishRun(ctx, runID, script.RunStatusError, errMsg, nil); err != nil {
			r.logger.Error("finish error run", "run_id", runID, "err", err)
		}
	}
	return nil
}

// execute runs the supplied JS source under the timeout guard and translates
// the result into a (status, errMsg) pair. A panic from inside Go-backed
// bindings is recovered here so it surfaces as RunStatusError rather than
// crashing the host process.
func (r *Runner) execute(rt *goja.Runtime, code string) (status script.RunStatus, errMsg string) {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	guard := time.AfterFunc(timeout, func() {
		rt.Interrupt("timeout")
	})
	defer guard.Stop()

	defer func() {
		if rec := recover(); rec != nil {
			status = script.RunStatusError
			errMsg = fmt.Sprintf("runtime panic: %v", rec)
			r.logger.Error("runtime panic", "err", rec)
		}
	}()

	_, err := rt.RunString(code)
	if err == nil {
		return script.RunStatusOK, ""
	}
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		// Interrupt value carries the marker we passed to rt.Interrupt;
		// the only marker we use today is "timeout".
		return script.RunStatusTimeout, fmt.Sprintf("timeout after %s", timeout)
	}
	return script.RunStatusError, err.Error()
}

// loadStateBuffer reads scripts.user_state and decodes it. A corrupt or
// empty blob falls back to an empty map so the runtime never blocks on
// historical bad data — the user can overwrite it on the next ok run.
func (r *Runner) loadStateBuffer(ctx context.Context, scriptID int64) *stateBuffer {
	raw, err := r.scripts.ReadUserState(ctx, scriptID)
	if err != nil {
		r.logger.Error("read user_state", "script_id", scriptID, "err", err)
		return newStateBuffer(nil)
	}
	if len(raw) == 0 {
		return newStateBuffer(nil)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		r.logger.Error("parse user_state", "script_id", scriptID, "err", err)
		return newStateBuffer(nil)
	}
	return newStateBuffer(m)
}

// flushQueue persists every buffered queueTask call. Each entry's tags are
// resolved (auto-create) and attached. Persistence is best-effort: a single
// failed task is logged but does not abort the rest, so a partially-spawned
// batch is preferable to nothing.
func (r *Runner) flushQueue(ctx context.Context, scriptID int64, q *taskQueue) []int64 {
	items := q.Drain()
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	sid := scriptID
	for _, it := range items {
		tagIDs, err := r.tags.Resolve(ctx, it.Tags, true)
		if err != nil {
			r.logger.Error("resolve tags", "script_id", scriptID, "err", err)
			continue
		}
		created, err := r.tasks.Create(ctx, task.CreateInput{
			Title:             it.Title,
			Notes:             it.Notes,
			DueDate:           it.DueDate,
			Tags:              it.Tags,
			SpawnedByScriptID: &sid,
		})
		if err != nil {
			r.logger.Error("create spawned task", "script_id", scriptID, "err", err)
			continue
		}
		if len(tagIDs) > 0 {
			if err := r.tasks.SetTagsByID(ctx, created.ID, tagIDs); err != nil {
				r.logger.Error("set tags on spawned task", "task_id", created.ID, "err", err)
			}
		}
		ids = append(ids, created.ID)
	}
	return ids
}

// persistState writes the flushed state buffer back to scripts.user_state.
// A marshal failure is logged and the row is left untouched.
func (r *Runner) persistState(ctx context.Context, scriptID int64, buf *stateBuffer) {
	blob, err := json.Marshal(buf.Flush())
	if err != nil {
		r.logger.Error("marshal user_state", "script_id", scriptID, "err", err)
		return
	}
	if err := r.scripts.WriteUserState(ctx, scriptID, blob); err != nil {
		r.logger.Error("write user_state", "script_id", scriptID, "err", err)
	}
}

// failRun is a small helper for the rare case where the runtime can't even
// set up the ctx surface (e.g. goja itself errored out). It stamps the run
// row with an error so the UI shows a meaningful state.
func (r *Runner) failRun(ctx context.Context, runID int64, msg string) {
	if err := r.scripts.FinishRun(ctx, runID, script.RunStatusError, msg, nil); err != nil {
		r.logger.Error("finish failed-setup run", "run_id", runID, "err", err)
	}
}

// exampleRunner documents the consumer-side interface contract.
//
// Per the project-wide dependency rule (consumer declares the interface),
// downstream packages — the scheduler in internal/scheduler and the HTTP
// API in internal/http — each declare their own narrow Runner interface.
// *runtime.Runner satisfies those declarations structurally.
//
// The canonical shape is:
//
//	type Runner interface {
//	    Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
//	}
//
// The compile-time assertion below pins this shape so an accidental
// signature change here breaks the build immediately rather than silently
// breaking consumers at link time.
type exampleRunner interface {
	Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
}

var _ exampleRunner = (*Runner)(nil)
