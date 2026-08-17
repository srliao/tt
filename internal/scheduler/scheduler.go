// Package scheduler drives periodic script execution. A single ticker
// goroutine sweeps script.Service.DueAt(now) every interval and enqueues
// matching scripts onto a bounded channel. A single worker goroutine
// consumes that channel sequentially: for scheduled jobs it creates the
// script_runs row inline (so a single in-flight scheduled run exists at a
// time) and then hands off to a runtime.Runner. Manual triggers reuse the
// same queue but the caller (HTTP handler) creates the run row up front.
//
// Per spec §5 the scheduler is intentionally single-worker: user scripts
// should not race each other. Queue overflow on scheduled triggers is
// silently dropped (with a counter); manual triggers return
// ErrSchedulerBusy so HTTP can map to 503.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/srliao/tt/internal/script"
)

// defaultInterval is the spec-mandated 15-minute scheduler cadence. Tests
// override this via WithInterval.
const defaultInterval = 15 * time.Minute

// defaultQueueSize is the bounded depth of the shared scheduled/manual
// queue. Sized so a burst of scripts plus a few manual triggers fit
// without surprising backpressure.
const defaultQueueSize = 100

// ErrSchedulerBusy is returned by EnqueueManual when the queue is full.
// HTTP handlers should translate this into 503 Service Unavailable.
var ErrSchedulerBusy = errors.New("scheduler busy")

// Runner is the narrow consumer-side interface the scheduler depends on.
// *runtime.Runner satisfies it structurally.
type Runner interface {
	Run(ctx context.Context, scriptID, runID int64, trigger script.Trigger) error
}

// ScriptLookup is the narrow slice of script.Service the scheduler needs.
// Keeping it separate decouples the scheduler from CRUD/log methods and
// makes test stubs trivial.
type ScriptLookup interface {
	DueAt(ctx context.Context, now time.Time, loc *time.Location) ([]script.Script, error)
	RecoverOrphanedRuns(ctx context.Context) error
	StartRun(ctx context.Context, scriptID int64, trigger script.Trigger) (script.Run, error)
}

// job is the message exchanged between the ticker/HTTP-handler and the
// worker. runID is zero for scheduled jobs (the worker calls StartRun) and
// non-zero for manual jobs (the caller pre-created the row).
type job struct {
	scriptID int64
	runID    int64
	trigger  script.Trigger
}

// Scheduler owns the ticker + worker goroutines and the bounded job
// queue. Construct with New and drive with Start / Stop.
type Scheduler struct {
	runner   Runner
	scripts  ScriptLookup
	clock    func() time.Time
	loc      *time.Location
	interval time.Duration
	queue    chan job
	logger   *slog.Logger
	dropped  atomic.Int64
	stop     chan struct{}
	done     sync.WaitGroup
	stopOnce sync.Once
}

// Option mutates a *Scheduler during construction.
type Option func(*Scheduler)

// WithInterval overrides the default 15-minute tick cadence. Intended for
// tests that need either fast feedback or, conversely, a long interval so
// the periodic tick never fires during a single test.
func WithInterval(d time.Duration) Option {
	return func(s *Scheduler) {
		if d > 0 {
			s.interval = d
		}
	}
}

// WithClock overrides the function used to obtain the current time when
// the ticker performs a sweep. Defaults to time.Now.
func WithClock(f func() time.Time) Option {
	return func(s *Scheduler) {
		if f != nil {
			s.clock = f
		}
	}
}

// WithLocation sets the timezone in which schedules resolve calendar days —
// which weekday it is, which day of the month, and whether a script already
// ran "today". Defaults to UTC; production passes config.Config.Location.
// A nil location is ignored so the default can't be accidentally cleared.
func WithLocation(loc *time.Location) Option {
	return func(s *Scheduler) {
		if loc != nil {
			s.loc = loc
		}
	}
}

// WithQueueSize overrides the default queue depth. Sizes <= 0 are
// ignored so callers can't accidentally disable the buffer.
func WithQueueSize(n int) Option {
	return func(s *Scheduler) {
		if n > 0 {
			s.queue = make(chan job, n)
		}
	}
}

// New constructs a Scheduler bound to the supplied runner, lookup, and
// logger. The scheduler does not start until Start is called.
func New(runner Runner, scripts ScriptLookup, logger *slog.Logger, opts ...Option) *Scheduler {
	s := &Scheduler{
		runner:   runner,
		scripts:  scripts,
		clock:    time.Now,
		loc:      time.UTC,
		interval: defaultInterval,
		queue:    make(chan job, defaultQueueSize),
		logger:   logger,
		stop:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start performs the startup sweep (orphan recovery + an immediate DueAt
// pass) and launches the ticker and worker goroutines. Start is a one-shot
// method; calling it twice will leak the second pair of goroutines.
//
// Failures inside RecoverOrphanedRuns and the initial DueAt sweep are
// logged but do not prevent the scheduler from coming online — a transient
// DB hiccup at boot shouldn't keep the binary from ever scheduling
// anything.
func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.scripts.RecoverOrphanedRuns(ctx); err != nil {
		s.logger.Error("scheduler: recover orphaned runs", "err", err)
	}

	s.sweep(ctx)

	s.done.Add(2)
	go s.runTicker(ctx)
	go s.runWorker(ctx)
	return nil
}

// Stop signals both goroutines to exit and blocks until they do. Safe to
// call more than once; subsequent calls are no-ops.
//
// We deliberately do not close(s.queue). EnqueueManual is called from HTTP
// handler goroutines that have no synchronization with Stop, and the ticker
// goroutine could be in the middle of a sweep — closing the queue would
// expose those callers to a send-on-closed-channel panic. Instead, producers
// observe s.stop and skip the send; the worker selects on both s.stop and
// s.queue so it exits even with buffered jobs.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
	s.done.Wait()
}

// EnqueueManual pushes a pre-created run onto the queue. The caller must
// have already invoked script.Service.StartRun (so the HTTP handler can
// return {run_id} immediately). When the queue is full this returns
// ErrSchedulerBusy so callers can map to HTTP 503.
func (s *Scheduler) EnqueueManual(scriptID, runID int64) error {
	return s.tryPush(job{scriptID: scriptID, runID: runID, trigger: script.TriggerManual})
}

// Dropped returns the number of scheduled jobs that were dropped due to
// queue overflow. Exposed for tests and future observability.
func (s *Scheduler) Dropped() int64 {
	return s.dropped.Load()
}

// enqueueScheduled is the ticker-side push. It always has runID=0 — the
// worker creates the script_runs row right before invoking the runtime so
// exactly one in-flight scheduled run exists per script.
func (s *Scheduler) enqueueScheduled(scriptID int64) {
	_ = s.tryPush(job{scriptID: scriptID, trigger: script.TriggerScheduled})
}

// tryPush is the shared non-blocking enqueue. Returns ErrSchedulerBusy
// for manual triggers on full queue; for scheduled triggers it increments
// the drop counter, logs, and returns nil so the ticker keeps marching.
//
// After Stop closes s.stop, producers must not send on s.queue (the worker
// is exiting; sends would block forever and Stop would deadlock on
// done.Wait). Manual callers get ErrSchedulerBusy so HTTP can map to 503;
// scheduled pushes are silently discarded since the ticker is itself about
// to exit.
func (s *Scheduler) tryPush(j job) error {
	select {
	case <-s.stop:
		if j.trigger == script.TriggerManual {
			return ErrSchedulerBusy
		}
		return nil
	default:
	}
	select {
	case s.queue <- j:
		return nil
	case <-s.stop:
		if j.trigger == script.TriggerManual {
			return ErrSchedulerBusy
		}
		return nil
	default:
		if j.trigger == script.TriggerManual {
			return ErrSchedulerBusy
		}
		s.dropped.Add(1)
		s.logger.Warn("scheduler queue full; dropping scheduled run", "script_id", j.scriptID)
		return nil
	}
}

// sweep is one DueAt pass plus enqueue of every matching script. Called
// from Start (initial sweep) and from the ticker loop. Errors are logged;
// the scheduler retries on the next tick.
func (s *Scheduler) sweep(ctx context.Context) {
	scripts, err := s.scripts.DueAt(ctx, s.clock(), s.loc)
	if err != nil {
		s.logger.Error("scheduler: DueAt", "err", err)
		return
	}
	for _, sc := range scripts {
		s.enqueueScheduled(sc.ID)
	}
}
