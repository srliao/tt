package scheduler_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srliao/tt/internal/scheduler"
	"github.com/srliao/tt/internal/script"
)

// stubLookup is an in-memory ScriptLookup for the scheduler tests. It records
// every call so the tests can assert on ordering and arguments without
// touching the real database.
type stubLookup struct {
	mu sync.Mutex

	due      []script.Script
	dueErr   error
	dueCalls int
	dueLocs  []*time.Location

	startErr     error
	startCalls   []startCall
	runIDCounter int64

	recoverErr   error
	recoverCalls int
}

type startCall struct {
	scriptID int64
	trigger  script.Trigger
}

func newStubLookup() *stubLookup {
	return &stubLookup{}
}

func (s *stubLookup) DueAt(_ context.Context, _ time.Time, loc *time.Location) ([]script.Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dueCalls++
	s.dueLocs = append(s.dueLocs, loc)
	if s.dueErr != nil {
		return nil, s.dueErr
	}
	out := make([]script.Script, len(s.due))
	copy(out, s.due)
	return out, nil
}

func (s *stubLookup) RecoverOrphanedRuns(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoverCalls++
	return s.recoverErr
}

func (s *stubLookup) StartRun(_ context.Context, scriptID int64, trigger script.Trigger) (script.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls = append(s.startCalls, startCall{scriptID: scriptID, trigger: trigger})
	if s.startErr != nil {
		return script.Run{}, s.startErr
	}
	s.runIDCounter++
	return script.Run{ID: s.runIDCounter, ScriptID: scriptID, Trigger: trigger}, nil
}

func (s *stubLookup) snapshot() (int, int, []startCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]startCall, len(s.startCalls))
	copy(cp, s.startCalls)
	return s.dueCalls, s.recoverCalls, cp
}

// stubRunner records each invocation. If block is non-nil the runner blocks
// on it before returning, which the queue-overflow tests use to pin jobs in
// flight.
type stubRunner struct {
	mu    sync.Mutex
	calls []runnerCall

	block   chan struct{} // if non-nil, Run waits on this before returning
	err     error
	panicOn func(runCall runnerCall) bool

	count atomic.Int64
}

type runnerCall struct {
	scriptID int64
	runID    int64
	trigger  script.Trigger
}

func newStubRunner() *stubRunner {
	return &stubRunner{}
}

func (r *stubRunner) Run(_ context.Context, scriptID, runID int64, trigger script.Trigger) error {
	rc := runnerCall{scriptID: scriptID, runID: runID, trigger: trigger}
	r.mu.Lock()
	r.calls = append(r.calls, rc)
	panicOn := r.panicOn
	block := r.block
	err := r.err
	r.mu.Unlock()

	r.count.Add(1)
	if panicOn != nil && panicOn(rc) {
		panic("stubRunner: induced panic")
	}
	if block != nil {
		<-block
	}
	return err
}

func (r *stubRunner) snapshot() []runnerCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]runnerCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// silentLogger returns a *slog.Logger that discards everything, so tests
// don't pollute test output.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitFor polls fn until it returns true or the deadline elapses.
func waitFor(t *testing.T, d time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waitFor: condition not met within %s", d)
}

func TestStart_RunsRecoverThenInitialSweep(t *testing.T) {
	scripts := newStubLookup()
	scripts.due = []script.Script{{ID: 7}}
	runner := newStubRunner()

	// Long interval so the periodic tick never fires during the test —
	// only the initial sweep should happen.
	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(1*time.Hour),
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	waitFor(t, 2*time.Second, func() bool {
		return runner.count.Load() >= 1
	})

	dueCalls, recoverCalls, startCalls := scripts.snapshot()
	if recoverCalls != 1 {
		t.Fatalf("RecoverOrphanedRuns calls = %d, want 1", recoverCalls)
	}
	if dueCalls < 1 {
		t.Fatalf("DueAt calls = %d, want >= 1", dueCalls)
	}
	if len(startCalls) != 1 || startCalls[0].scriptID != 7 || startCalls[0].trigger != script.TriggerScheduled {
		t.Fatalf("startCalls = %+v, want one call for script 7 scheduled", startCalls)
	}

	calls := runner.snapshot()
	if len(calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(calls))
	}
	if calls[0].scriptID != 7 || calls[0].trigger != script.TriggerScheduled || calls[0].runID == 0 {
		t.Fatalf("runner call = %+v, want scriptID=7 trigger=scheduled non-zero runID", calls[0])
	}
}

func TestEnqueueScheduled_CallsStartRunThenRunner(t *testing.T) {
	scripts := newStubLookup()
	scripts.due = []script.Script{{ID: 42}}
	runner := newStubRunner()

	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(1*time.Hour),
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	waitFor(t, 2*time.Second, func() bool {
		return runner.count.Load() >= 1
	})

	_, _, startCalls := scripts.snapshot()
	if len(startCalls) != 1 || startCalls[0].scriptID != 42 || startCalls[0].trigger != script.TriggerScheduled {
		t.Fatalf("startCalls = %+v, want one scheduled call for script 42", startCalls)
	}
	calls := runner.snapshot()
	if len(calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(calls))
	}
	// stubLookup hands out runID=1 first.
	if calls[0].runID != 1 {
		t.Fatalf("runner runID = %d, want 1 (from StartRun)", calls[0].runID)
	}
	if calls[0].trigger != script.TriggerScheduled {
		t.Fatalf("runner trigger = %q, want scheduled", calls[0].trigger)
	}
}

func TestEnqueueManual_SkipsStartRun(t *testing.T) {
	scripts := newStubLookup()
	// No due scripts so the initial sweep is a no-op.
	runner := newStubRunner()

	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(1*time.Hour),
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	if err := s.EnqueueManual(99, 42); err != nil {
		t.Fatalf("EnqueueManual: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		return runner.count.Load() >= 1
	})

	_, _, startCalls := scripts.snapshot()
	if len(startCalls) != 0 {
		t.Fatalf("StartRun called %d times; manual triggers must not call StartRun: %+v", len(startCalls), startCalls)
	}
	calls := runner.snapshot()
	if len(calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(calls))
	}
	if calls[0].scriptID != 99 || calls[0].runID != 42 || calls[0].trigger != script.TriggerManual {
		t.Fatalf("runner call = %+v, want scriptID=99 runID=42 trigger=manual", calls[0])
	}
}

func TestStop_GoroutinesExit(t *testing.T) {
	scripts := newStubLookup()
	runner := newStubRunner()
	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(50*time.Millisecond),
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Stop did not return within 2s; goroutines stuck")
	}

	// Double Stop must not panic.
	s.Stop()
}

func TestQueueOverflow_ScheduledDropped(t *testing.T) {
	scripts := newStubLookup()
	scripts.due = []script.Script{{ID: 11}, {ID: 12}, {ID: 13}, {ID: 14}}
	runner := newStubRunner()
	runner.block = make(chan struct{})

	// Tiny queue so the very first scheduled sweep overflows: the worker
	// picks up one job and blocks in runner.Run; the next slot fills the
	// buffer; everything beyond is dropped.
	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(1*time.Hour),
		scheduler.WithQueueSize(1),
	)
	// Defer order matters: unblock runner BEFORE Stop so the worker can
	// drain the queue and exit. Stop is registered second so it runs first.
	defer func() {
		close(runner.block)
	}()
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Register Stop AFTER the unblock so LIFO ordering closes the block
	// channel first, letting Stop's worker.Wait() return.
	t.Cleanup(s.Stop)

	// Wait for the initial sweep to drop at least one scheduled job.
	waitFor(t, 2*time.Second, func() bool {
		return s.Dropped() >= 1
	})

	if got := s.Dropped(); got < 1 {
		t.Fatalf("Dropped = %d, want >= 1", got)
	}
}

func TestQueueOverflow_ManualReturnsErrSchedulerBusy(t *testing.T) {
	scripts := newStubLookup()
	runner := newStubRunner()
	runner.block = make(chan struct{})

	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(1*time.Hour),
		scheduler.WithQueueSize(2),
	)
	// Unblock runner before Stop: defer runs first (close), then Cleanup
	// (Stop) runs after, so the worker can drain and exit cleanly.
	defer func() { close(runner.block) }()
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)

	if err := s.EnqueueManual(1, 100); err != nil {
		t.Fatalf("EnqueueManual #1: %v", err)
	}
	waitFor(t, 1*time.Second, func() bool { return runner.count.Load() >= 1 })

	if err := s.EnqueueManual(2, 101); err != nil {
		t.Fatalf("EnqueueManual #2: %v", err)
	}
	if err := s.EnqueueManual(3, 102); err != nil {
		t.Fatalf("EnqueueManual #3: %v", err)
	}
	// Queue is now full (2 waiting + 1 in-flight). Next manual must error.
	err := s.EnqueueManual(4, 103)
	if !errors.Is(err, scheduler.ErrSchedulerBusy) {
		t.Fatalf("EnqueueManual on full queue = %v, want ErrSchedulerBusy", err)
	}
}

// TestEnqueueManualConcurrentWithStop verifies that the scheduler doesn't
// panic with "send on closed channel" when HTTP handler goroutines hammer
// EnqueueManual while Stop is running.
func TestEnqueueManualConcurrentWithStop(t *testing.T) {
	scripts := newStubLookup()
	runner := newStubRunner()
	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(1*time.Hour),
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				// Errors are fine post-Stop; the only failure mode we're
				// guarding against is a runtime panic.
				_ = s.EnqueueManual(int64(i), int64(j))
			}
		}(i)
	}
	// Let some sends land, then stop concurrently with the producers.
	time.Sleep(5 * time.Millisecond)
	s.Stop()
	wg.Wait()
}

func TestRunnerPanicRecovered(t *testing.T) {
	scripts := newStubLookup()
	runner := newStubRunner()
	// Panic exactly once: on the first call.
	var panicked atomic.Bool
	runner.panicOn = func(c runnerCall) bool {
		if panicked.Load() {
			return false
		}
		panicked.Store(true)
		return true
	}

	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(1*time.Hour),
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	// First manual triggers the panic, second proves the worker survived.
	if err := s.EnqueueManual(1, 100); err != nil {
		t.Fatalf("EnqueueManual #1: %v", err)
	}
	waitFor(t, 1*time.Second, func() bool { return runner.count.Load() >= 1 })

	if err := s.EnqueueManual(2, 200); err != nil {
		t.Fatalf("EnqueueManual #2: %v", err)
	}
	waitFor(t, 1*time.Second, func() bool { return runner.count.Load() >= 2 })

	calls := runner.snapshot()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(calls))
	}
	if calls[1].runID != 200 {
		t.Fatalf("second runner call runID = %d, want 200", calls[1].runID)
	}
}

// TestSchedulerPassesLocationToDueAt pins that the configured app timezone
// reaches the schedule matcher — without it, DueAt would evaluate calendar
// days in UTC no matter how the binary was configured.
func TestSchedulerPassesLocationToDueAt(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	scripts := newStubLookup()
	runner := newStubRunner()
	s := scheduler.New(runner, scripts, silentLogger(),
		scheduler.WithInterval(time.Hour),
		scheduler.WithLocation(ny),
	)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	scripts.mu.Lock()
	locs := append([]*time.Location(nil), scripts.dueLocs...)
	scripts.mu.Unlock()

	if len(locs) == 0 {
		t.Fatalf("DueAt was never called")
	}
	for i, loc := range locs {
		if loc != ny {
			t.Errorf("DueAt call %d location = %v, want %v", i, loc, ny)
		}
	}
}

// TestSchedulerDefaultsToUTC guards the zero-config path: a scheduler built
// without WithLocation must hand DueAt a usable location, not nil.
func TestSchedulerDefaultsToUTC(t *testing.T) {
	scripts := newStubLookup()
	runner := newStubRunner()
	s := scheduler.New(runner, scripts, silentLogger(), scheduler.WithInterval(time.Hour))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	scripts.mu.Lock()
	locs := append([]*time.Location(nil), scripts.dueLocs...)
	scripts.mu.Unlock()

	if len(locs) == 0 {
		t.Fatalf("DueAt was never called")
	}
	if locs[0] != time.UTC {
		t.Errorf("default DueAt location = %v, want UTC", locs[0])
	}
}
