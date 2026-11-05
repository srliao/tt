package script_test

import (
	"context"
	"testing"
	"time"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/script"
)

// fakeClock is a settable clock so a test can move time between calls.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestServiceStampsTimestampsFromClock(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)}
	svc := script.New(dbtest.New(t), script.WithClock(clk.now))

	sc, err := svc.Create(ctx, script.CreateInput{
		Name:     "s",
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !sc.CreatedAt.Equal(clk.t) || !sc.UpdatedAt.Equal(clk.t) {
		t.Fatalf("create stamps = %v / %v, want %v", sc.CreatedAt, sc.UpdatedAt, clk.t)
	}

	clk.t = time.Date(2031, 3, 5, 0, 0, 0, 0, time.UTC)
	up, err := svc.Update(ctx, sc.ID, script.UpdateInput{
		Name:     "s2",
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !up.UpdatedAt.Equal(clk.t) {
		t.Errorf("update updated_at = %v, want %v", up.UpdatedAt, clk.t)
	}

	clk.t = time.Date(2031, 3, 6, 0, 0, 0, 0, time.UTC)
	if err := svc.WriteUserState(ctx, sc.ID, []byte(`{"n":1}`)); err != nil {
		t.Fatalf("WriteUserState: %v", err)
	}
	got, err := svc.Get(ctx, sc.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.UpdatedAt.Equal(clk.t) {
		t.Errorf("user state updated_at = %v, want %v", got.UpdatedAt, clk.t)
	}

	clk.t = time.Date(2031, 3, 7, 0, 0, 0, 0, time.UTC)
	started := clk.t
	run, err := svc.StartRun(ctx, sc.ID, script.TriggerManual)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if !run.StartedAt.Equal(started) {
		t.Errorf("started_at = %v, want %v", run.StartedAt, started)
	}

	clk.t = time.Date(2031, 3, 7, 0, 0, 1, 0, time.UTC)
	if err := svc.AppendLog(ctx, run.ID, script.LogInfo, "hi"); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}
	logs, err := svc.GetLogs(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 1 || !logs[0].LoggedAt.Equal(clk.t) {
		t.Errorf("logs = %+v, want one logged at %v", logs, clk.t)
	}

	clk.t = time.Date(2031, 3, 7, 0, 0, 2, 0, time.UTC)
	if err := svc.FinishRun(ctx, run.ID, script.RunStatusOK, "", nil); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	finished, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if finished.FinishedAt == nil || !finished.FinishedAt.Equal(clk.t) {
		t.Errorf("finished_at = %v, want %v", finished.FinishedAt, clk.t)
	}
	if !finished.StartedAt.Equal(started) {
		t.Errorf("finish changed started_at to %v", finished.StartedAt)
	}
}

func TestRecoverOrphanedRunsStampsClock(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2031, 3, 4, 0, 0, 0, 0, time.UTC)}
	svc := script.New(dbtest.New(t), script.WithClock(clk.now))

	sc, err := svc.Create(ctx, script.CreateInput{
		Name:     "s",
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	run, err := svc.StartRun(ctx, sc.ID, script.TriggerScheduled)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	clk.t = time.Date(2031, 3, 4, 1, 0, 0, 0, time.UTC)
	if err := svc.RecoverOrphanedRuns(ctx); err != nil {
		t.Fatalf("RecoverOrphanedRuns: %v", err)
	}
	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(clk.t) {
		t.Errorf("finished_at = %v, want %v", got.FinishedAt, clk.t)
	}
}
