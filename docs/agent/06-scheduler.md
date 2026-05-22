# Scheduler

Background ticker + single worker goroutine. Drives both scheduled and manual script runs through one queue.

Files: `internal/scheduler/scheduler.go`, `internal/scheduler/worker.go`.

## Constants

| Constant | Value | Override |
|---|---|---|
| `defaultInterval` | 15 minutes | `WithInterval(d)` |
| `defaultQueueSize` | 100 | `WithQueueSize(n)` |
| (no per-job timeout — that's the runtime's job) | | |

## Topology

```
┌────────────┐                ┌──────────────┐
│  Ticker    │ enqueueScheduled│              │
│  goroutine ├───────────────►│              │
│  (sweep on │                │   Queue      │
│   tick)    │                │   chan job   │   ┌────────────┐
└────────────┘                │   (size 100) ├──►│  Worker    │
                              │              │   │  goroutine │
┌────────────┐ EnqueueManual  │              │   │ (1-at-a-   │
│  HTTP /run ├───────────────►│              │   │  time)     │
│  handler   │                │              │   └────────────┘
└────────────┘                └──────────────┘
```

## Job flow

`job{scriptID, runID, trigger}`:

- **Scheduled jobs**: `runID = 0` at enqueue. Worker calls `StartRun` right before invoking the runtime — guarantees a single in-flight scheduled run per script (ticker can't fan out across script duplicates).
- **Manual jobs**: HTTP handler pre-creates the run row via `StartRun` so the response can return `{"run_id": N}` immediately. Worker just calls `runner.Run`.

## Backpressure

`tryPush` is non-blocking. On full queue:

- `TriggerManual` → return `ErrSchedulerBusy`. HTTP maps to **503** with code `scheduler_busy`. The handler also calls `FinishRun(error, "scheduler busy")` so the pre-created row never sits in `running`.
- `TriggerScheduled` → silently dropped, `Dropped()` counter incremented, log line.

After `Stop()` closes `s.stop`, producers observe it and return immediately so post-shutdown enqueues don't deadlock or panic. **`s.queue` is intentionally NOT closed** — HTTP handler goroutines have no sync with Stop.

## Startup sequence

```
Start(ctx):
  1. RecoverOrphanedRuns()   ── mark any leftover 'running' rows as error
  2. sweep(ctx)              ── immediate DueAt pass (handles missed work)
  3. spawn ticker goroutine  ── 15-min sweep cadence
  4. spawn worker goroutine  ── consume queue forever
```

Errors in steps 1-2 are logged but do NOT prevent the scheduler from coming online — a transient DB hiccup shouldn't keep the binary from scheduling anything.

## Consumer-side interfaces

```go
// internal/scheduler/scheduler.go
type Runner interface {
    Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
}
type ScriptLookup interface {
    DueAt(ctx context.Context, now time.Time) ([]script.Script, error)
    RecoverOrphanedRuns(ctx context.Context) error
    StartRun(ctx context.Context, scriptID int64, trigger script.Trigger) (script.Run, error)
}
```

Only what's needed. `*runtime.Runner` satisfies `Runner` structurally; `*script.Impl` satisfies `ScriptLookup`.

## Panic recovery

`processJob` defers a `recover()` so a panic in `StartRun` or `runner.Run` is logged with `script_id`, `run_id`, `trigger`, and the worker keeps running.

## Stopping

`Stop()` is idempotent (`sync.Once`):
1. close `s.stop` → ticker exits at next select, producers stop pushing.
2. `s.done.Wait()` blocks until both goroutines have returned.

Buffered jobs at shutdown are discarded. Orphan recovery on next boot handles any rows left in `running`.
