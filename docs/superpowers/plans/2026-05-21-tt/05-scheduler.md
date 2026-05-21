# Phase 05 — Background Scheduler

> Read `00-index.md` first. Commit after each task.

**Goal:** A 15-minute ticker drives `script.Service.DueAt(now)`, enqueueing each matching script into a buffered channel that a single worker goroutine consumes by calling `Runner.Run`. Startup sweep marks orphaned runs and triggers any scripts that should already have fired.

**Dependencies:** Phase 03c (script service), Phase 04 (`Runner`).

**Tech stack:** Go stdlib (`context`, `time`, `sync`), `slog`.

**Parallelizable with:** 06, 07.

## File map

```
internal/scheduler/
├── scheduler.go        # Scheduler struct + Start/Stop
├── worker.go           # worker goroutine consuming the queue
├── scheduler_test.go
└── worker_test.go
```

## Background (spec invariants)

- Tick interval: **15 minutes** (configurable for tests).
- Worker count: **1**. Sequential execution; no parallelism (spec §5 "Concurrency model").
- Queue: buffered channel, size **100**.
  - Scheduled triggers that hit a full queue are **dropped + logged**.
  - Manual triggers that hit a full queue return an error so HTTP can respond `503`.
- Manual and scheduled triggers share the same queue.
- On startup: call `script.Service.RecoverOrphanedRuns()`, then run an immediate `DueAt(now)` sweep (so missed work after a binary restart fires once).
- `last_run_at` is updated by the **runtime**, not the scheduler.
- **Run row ownership.** For **scheduled triggers**, the scheduler calls `script.StartRun(scriptID, "scheduled")` *inside the worker goroutine* right before invoking the runtime, so a single in-flight scheduled run exists at a time. For **manual triggers**, the caller (HTTP handler) creates the run row *before* enqueueing — the job carries the existing `runID`. This lets the HTTP `POST /scripts/:id/run` response return `{run_id}` immediately.

## Task 1: Scheduler struct + start/stop — failing test

**Files:** `internal/scheduler/scheduler_test.go`

- [ ] **Tests cover:**
  - Constructing a `Scheduler` with a fake clock and a stub `runner` (records calls). Starting it: a tick triggers `DueAt(fakeNow)`; for each due script, the queue gets an enqueue; worker calls `scripts.StartRun(scriptID, "scheduled")` to obtain a `runID`, then calls `runner.Run(ctx, scriptID, runID, "scheduled")`.
  - Stopping the scheduler causes the ticker goroutine and worker goroutine to exit within a short timeout.
  - Manual enqueue via `EnqueueManual(scriptID, runID)` reaches the worker without calling `StartRun` again (the caller has already created the run row); worker calls `runner.Run(ctx, scriptID, runID, "manual")`.
  - Queue overflow on scheduled trigger: pre-fill the queue with 100 items; the next scheduled enqueue is dropped. Use a `dropped int64` counter that tests can read.
  - Queue overflow on manual trigger: with queue full, `EnqueueManual(...)` returns `ErrSchedulerBusy`.
- [ ] Use a fake `script.Service` that returns a fixed `[]Script` from `DueAt`, returns a stub `Run` from `StartRun` (so the test can assert the runID is propagated), and a stub `Runner` (`type runnerFunc func(ctx, scriptID, runID int64, trigger script.Trigger) error`).
- [ ] Run → undefined.

## Task 2: Scheduler — implementation

**Files:** `internal/scheduler/scheduler.go`, `internal/scheduler/worker.go`

- [ ] Declare a narrow consumer-side `Runner` interface in `scheduler.go`:
  ```go
  type Runner interface {
      Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
  }
  ```

- [ ] Declare a narrow `ScriptLookup` interface so the scheduler is decoupled from the full `script.Service`:
  ```go
  type ScriptLookup interface {
      DueAt(ctx context.Context, now time.Time) ([]script.Script, error)
      RecoverOrphanedRuns(ctx context.Context) error
      StartRun(ctx context.Context, scriptID int64, trigger script.Trigger) (script.Run, error)
  }
  ```

- [ ] `Scheduler` struct holds: `runner Runner`, `scripts ScriptLookup`, `clock func() time.Time`, `interval time.Duration`, `queue chan job`, `logger *slog.Logger`, `dropped atomic.Int64`, `stop chan struct{}`, `done sync.WaitGroup`.

- [ ] `job` is:
  ```go
  type job struct {
      scriptID int64
      runID    int64       // 0 for scheduled jobs — worker creates the run row
      trigger  script.Trigger
  }
  ```

- [ ] `New(runner, scripts, logger, opts ...Option) *Scheduler`:
  - Default interval `15 * time.Minute`, queue size `100`, clock `time.Now`.
  - Options: `WithInterval(d)`, `WithClock(f)`, `WithQueueSize(n)`.

- [ ] `Start(ctx context.Context) error`:
  - Recover orphaned runs (`scripts.RecoverOrphanedRuns`).
  - Immediate sweep: `enqueueDue(scripts.DueAt(ctx, clock()))`.
  - Launch ticker goroutine (`done.Add(1)`); on each tick: `enqueueDue(...)`. Exits when `stop` closes or ctx cancels.
  - Launch worker goroutine (`done.Add(1)`): for each `j := range queue`:
    1. If `j.runID == 0` (scheduled), call `run, err := scripts.StartRun(ctx, j.scriptID, j.trigger)`; on error log+continue. `j.runID = run.ID`.
    2. Call `runner.Run(ctx, j.scriptID, j.runID, j.trigger)`. Recover panics, log, continue.

- [ ] Public methods:
  ```go
  // EnqueueScheduled is called by the ticker — runID is 0; worker creates the row.
  func (s *Scheduler) enqueueScheduled(scriptID int64) { ... }

  // EnqueueManual is called by the HTTP handler with a pre-created runID.
  // Returns ErrSchedulerBusy when the queue is full.
  func (s *Scheduler) EnqueueManual(scriptID, runID int64) error { ... }
  ```

  Both share this push helper:
  ```go
  select {
  case s.queue <- j:
      return nil
  default:
      if j.trigger == script.TriggerManual { return ErrSchedulerBusy }
      s.dropped.Add(1)
      s.logger.Warn("scheduler queue full; dropping scheduled run", "script_id", j.scriptID)
      return nil
  }
  ```

- [ ] `enqueueDue([]script.Script)`: loop, calling `enqueueScheduled(id)` for each.

- [ ] `Stop()` closes `stop` and `queue`, calls `done.Wait()`.

- [ ] `var ErrSchedulerBusy = errors.New("scheduler busy")`.

- [ ] Run scheduler tests → green.

- [ ] Commit:
  ```bash
  git add internal/scheduler/ && git commit -m "feat(scheduler): add ticker + worker with bounded queue"
  ```

## Task 3: Integration check (manual)

- [ ] Verify the full picture builds: `go build ./...`.
- [ ] No commit needed beyond the previous one.

## Phase completion checklist

- [ ] `go test ./internal/scheduler/... -v` all pass.
- [ ] `go build ./...` clean.
- [ ] Manual triggers return `ErrSchedulerBusy` when the queue is full; scheduled triggers are silently dropped (with an incrementing `dropped` counter).
- [ ] Startup performs orphan recovery before the first tick.
