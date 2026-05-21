package scheduler

import (
	"context"
	"time"
)

// runTicker drives periodic DueAt sweeps. The loop exits when either the
// stop channel closes (Stop was called) or ctx is cancelled. It does not
// close the queue; that is Stop's responsibility — the queue may still
// hold manual enqueues from concurrent HTTP handlers when the ticker exits.
func (s *Scheduler) runTicker(ctx context.Context) {
	defer s.done.Done()

	t := time.NewTicker(s.interval)
	defer t.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep(ctx)
		}
	}
}

// runWorker is the single consumer of the job queue. It executes one job
// at a time so concurrent script effects can never interleave. The loop
// exits when s.stop is closed (Stop was called); any jobs still buffered
// in s.queue at shutdown are discarded — they'll be picked up on next boot
// by the orphan-recovery + initial sweep path.
//
// Both StartRun and runner.Run are wrapped in panic recovery (per-job, via
// processJob) so a single misbehaving script can't take down the worker.
func (s *Scheduler) runWorker(ctx context.Context) {
	defer s.done.Done()

	for {
		select {
		case <-s.stop:
			return
		case <-ctx.Done():
			return
		case j := <-s.queue:
			s.processJob(ctx, j)
		}
	}
}

// processJob handles one queue entry. Pulled into its own method so the
// recover deferred inside runs per-job rather than once per worker
// lifetime.
func (s *Scheduler) processJob(ctx context.Context, j job) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("scheduler: worker recovered from panic",
				"script_id", j.scriptID,
				"run_id", j.runID,
				"trigger", string(j.trigger),
				"err", rec,
			)
		}
	}()

	if j.runID == 0 {
		// Scheduled job: create the run row in the worker so a single
		// in-flight scheduled run exists at a time.
		run, err := s.scripts.StartRun(ctx, j.scriptID, j.trigger)
		if err != nil {
			s.logger.Error("scheduler: start scheduled run",
				"script_id", j.scriptID,
				"err", err,
			)
			return
		}
		j.runID = run.ID
	}

	if err := s.runner.Run(ctx, j.scriptID, j.runID, j.trigger); err != nil {
		s.logger.Error("scheduler: runner returned error",
			"script_id", j.scriptID,
			"run_id", j.runID,
			"trigger", string(j.trigger),
			"err", err,
		)
	}
}
