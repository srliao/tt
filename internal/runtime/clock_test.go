package runtime_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/runtime"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

// A run whose services share one fake clock writes nothing but fake time:
// the run row, its logs, the spawned task and the script's state and
// last_run_at all carry the clock's instant.
func TestRunner_SharedClockStampsEveryWrite(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	clock := func() time.Time { return at }

	store := dbtest.New(t)
	tasks := task.New(store, task.WithClock(clock))
	scripts := script.New(store, script.WithClock(clock))
	r := runtime.New(tasks, tag.New(store), scripts,
		slog.New(slog.NewTextHandler(io.Discard, nil)), runtime.WithClock(clock))

	sc, err := scripts.Create(ctx, script.CreateInput{
		Name:     "s",
		Code:     `ctx.log("hi"); ctx.state.set("n", 1); ctx.queueTask({title: "spawned"});`,
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	run, err := scripts.StartRun(ctx, sc.ID, script.TriggerScheduled)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	at = time.Date(2031, 3, 4, 5, 15, 0, 0, time.UTC)
	if err := r.Run(ctx, sc.ID, run.ID, script.TriggerScheduled); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := scripts.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != script.RunStatusOK {
		t.Fatalf("status = %q (%s)", got.Status, got.ErrorMessage)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(at) {
		t.Errorf("finished_at = %v, want %v", got.FinishedAt, at)
	}
	logs, err := scripts.GetLogs(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 1 || !logs[0].LoggedAt.Equal(at) {
		t.Errorf("logs = %+v, want one logged at %v", logs, at)
	}
	if len(got.SpawnedTaskIDs) != 1 {
		t.Fatalf("spawned = %v, want 1", got.SpawnedTaskIDs)
	}
	spawned, err := tasks.Get(ctx, got.SpawnedTaskIDs[0])
	if err != nil {
		t.Fatalf("Get task: %v", err)
	}
	if !spawned.CreatedAt.Equal(at) {
		t.Errorf("spawned created_at = %v, want %v", spawned.CreatedAt, at)
	}
	after, err := scripts.Get(ctx, sc.ID)
	if err != nil {
		t.Fatalf("Get script: %v", err)
	}
	if after.LastRunAt == nil || !after.LastRunAt.Equal(at) {
		t.Errorf("last_run_at = %v, want %v", after.LastRunAt, at)
	}
	if !after.UpdatedAt.Equal(at) {
		t.Errorf("script updated_at = %v, want %v", after.UpdatedAt, at)
	}
}
