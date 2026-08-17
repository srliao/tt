package runtime_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/runtime"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

// runnerHarness wires up real services against an in-memory DB and gives
// every test a ready-to-use runner. The slog logger is silenced so test
// output stays focused on assertions.
type runnerHarness struct {
	t       *testing.T
	tasks   *task.Impl
	tags    *tag.Impl
	scripts *script.Impl
	runner  *runtime.Runner
}

func newHarness(t *testing.T, opts ...runtime.Option) *runnerHarness {
	t.Helper()
	store := dbtest.New(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tasks := task.New(store)
	tags := tag.New(store)
	scripts := script.New(store)
	r := runtime.New(tasks, tags, scripts, logger, opts...)
	return &runnerHarness{t: t, tasks: tasks, tags: tags, scripts: scripts, runner: r}
}

// createScript stashes a script row with the given code and returns its id.
func (h *runnerHarness) createScript(code string) int64 {
	h.t.Helper()
	sc, err := h.scripts.Create(context.Background(), script.CreateInput{
		Name:     "test",
		Code:     code,
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		h.t.Fatalf("create script: %v", err)
	}
	return sc.ID
}

// startRun obtains a run row in the running state. The runtime requires the
// caller to do this — Runner.Run does not create rows itself.
func (h *runnerHarness) startRun(scriptID int64, trigger script.Trigger) int64 {
	h.t.Helper()
	r, err := h.scripts.StartRun(context.Background(), scriptID, trigger)
	if err != nil {
		h.t.Fatalf("start run: %v", err)
	}
	return r.ID
}

func TestRunner_HappyPath_CreatesTaskAndOK(t *testing.T) {
	h := newHarness(t)
	sid := h.createScript(`ctx.queueTask({title: "hi", tags: ["weekly"]});`)
	rid := h.startRun(sid, script.TriggerManual)

	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}

	run, err := h.scripts.GetRun(context.Background(), rid)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != script.RunStatusOK {
		t.Fatalf("status = %q, want ok (err=%q)", run.Status, run.ErrorMessage)
	}
	if len(run.SpawnedTaskIDs) != 1 {
		t.Fatalf("spawned ids = %v, want 1", run.SpawnedTaskIDs)
	}
	got, err := h.tasks.Get(context.Background(), run.SpawnedTaskIDs[0])
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Title != "hi" {
		t.Fatalf("task title = %q, want hi", got.Title)
	}
	wantTags := map[string]bool{"weekly": true}
	for _, n := range got.Tags {
		delete(wantTags, n)
	}
	if len(wantTags) != 0 {
		t.Fatalf("missing tags on spawned task: %v (got %v)", wantTags, got.Tags)
	}
}

func TestRunner_LogsSurviveError(t *testing.T) {
	h := newHarness(t)
	sid := h.createScript(`ctx.log("before"); throw new Error("boom");`)
	rid := h.startRun(sid, script.TriggerManual)

	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}
	run, err := h.scripts.GetRun(context.Background(), rid)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != script.RunStatusError {
		t.Fatalf("status = %q, want error", run.Status)
	}
	if !strings.Contains(run.ErrorMessage, "boom") {
		t.Fatalf("error_message = %q, want contains boom", run.ErrorMessage)
	}
	logs, err := h.scripts.GetLogs(context.Background(), rid)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	foundBefore := false
	for _, l := range logs {
		if l.Message == "before" {
			foundBefore = true
		}
	}
	if !foundBefore {
		t.Fatalf("expected 'before' log entry, got %v", logs)
	}
	if len(run.SpawnedTaskIDs) != 0 {
		t.Fatalf("no tasks should have been created on error, got %v", run.SpawnedTaskIDs)
	}
}

func TestRunner_StateBuffersUntilOK(t *testing.T) {
	h := newHarness(t)
	// Error case: state should not persist.
	sid := h.createScript(`ctx.state.set("k", 1); throw new Error("nope");`)
	rid := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw, err := h.scripts.ReadUserState(context.Background(), sid)
	if err != nil {
		t.Fatalf("ReadUserState: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal user_state: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("user_state after error = %v, want empty", got)
	}

	// Success case: state should persist.
	sid2 := h.createScript(`ctx.state.set("k", 42);`)
	rid2 := h.startRun(sid2, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid2, rid2, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw2, err := h.scripts.ReadUserState(context.Background(), sid2)
	if err != nil {
		t.Fatalf("ReadUserState 2: %v", err)
	}
	var got2 map[string]any
	if err := json.Unmarshal(raw2, &got2); err != nil {
		t.Fatalf("unmarshal user_state 2: %v", err)
	}
	if v, _ := got2["k"].(float64); v != 42 {
		t.Fatalf("user_state.k = %v, want 42", got2["k"])
	}
}

// A spawned batch lands above everything already in the list, and reads in
// spawn order within itself: [a, b, c, <older task>].
func TestRunner_SpawnedBatchLandsAtTopInSpawnOrder(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	older, err := h.tasks.Create(ctx, task.CreateInput{Title: "older"})
	if err != nil {
		t.Fatalf("Create(older): %v", err)
	}

	sid := h.createScript(`
		ctx.queueTask({title: "a"});
		ctx.queueTask({title: "b"});
		ctx.queueTask({title: "c"});
	`)
	rid := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(ctx, sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := h.tasks.List(ctx, task.FilterSort{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	titles := make([]string, 0, len(got))
	for _, tk := range got {
		titles = append(titles, tk.Title)
	}
	want := []string{"a", "b", "c", "older"}
	if strings.Join(titles, ",") != strings.Join(want, ",") {
		t.Fatalf("list order = %v, want %v", titles, want)
	}
	if got[3].ID != older.ID {
		t.Fatalf("last row id = %d, want the pre-existing task %d", got[3].ID, older.ID)
	}
}

func TestRunner_LastSpawnPopulated(t *testing.T) {
	h := newHarness(t)
	// First run spawns a task.
	sid := h.createScript(`ctx.queueTask({title: "first"});`)
	rid := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Patch the script's code so the second run reads ctx.lastSpawn and
	// logs its serialized form.
	if _, err := h.scripts.Update(context.Background(), sid, script.UpdateInput{
		Name:     "test",
		Code:     `ctx.log(JSON.stringify(ctx.lastSpawn));`,
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	}); err != nil {
		t.Fatalf("update script: %v", err)
	}
	rid2 := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid, rid2, script.TriggerManual); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	logs, err := h.scripts.GetLogs(context.Background(), rid2)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected at least one log entry")
	}
	if !strings.Contains(logs[0].Message, `"title":"first"`) {
		t.Fatalf("lastSpawn log %q does not contain expected title", logs[0].Message)
	}
}

func TestRunner_LastSpawnsExposesEntireBatch(t *testing.T) {
	h := newHarness(t)
	// First run queues three tasks in one batch.
	sid := h.createScript(`
		ctx.queueTask({title: "a"});
		ctx.queueTask({title: "b"});
		ctx.queueTask({title: "c"});
	`)
	rid := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Second run reads ctx.lastSpawns. We log titles + lastSpawn.title so we
	// can assert both surfaces.
	if _, err := h.scripts.Update(context.Background(), sid, script.UpdateInput{
		Name: "test",
		Code: `
			ctx.log(ctx.lastSpawns.map(t => t.title).join(","));
			ctx.log(ctx.lastSpawn.title);
		`,
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	}); err != nil {
		t.Fatalf("update script: %v", err)
	}
	rid2 := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid, rid2, script.TriggerManual); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	logs, err := h.scripts.GetLogs(context.Background(), rid2)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected 2 log entries, got %d", len(logs))
	}
	if logs[0].Message != "a,b,c" {
		t.Fatalf("lastSpawns titles = %q, want %q", logs[0].Message, "a,b,c")
	}
	if logs[1].Message != "c" {
		t.Fatalf("lastSpawn.title = %q, want %q (last entry of batch)", logs[1].Message, "c")
	}
}

func TestRunner_Timeout(t *testing.T) {
	h := newHarness(t, runtime.WithTimeout(200*time.Millisecond))
	sid := h.createScript(`ctx.log("pre-loop"); while(true) {}`)
	rid := h.startRun(sid, script.TriggerManual)

	start := time.Now()
	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("Run took %s; expected sub-second timeout", elapsed)
	}

	run, err := h.scripts.GetRun(context.Background(), rid)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != script.RunStatusTimeout {
		t.Fatalf("status = %q, want timeout (err=%q)", run.Status, run.ErrorMessage)
	}
	if !strings.Contains(strings.ToLower(run.ErrorMessage), "timeout") {
		t.Fatalf("error_message = %q, want contains timeout", run.ErrorMessage)
	}
	if len(run.SpawnedTaskIDs) != 0 {
		t.Fatalf("no tasks should persist on timeout, got %v", run.SpawnedTaskIDs)
	}
	raw, err := h.scripts.ReadUserState(context.Background(), sid)
	if err != nil {
		t.Fatalf("ReadUserState: %v", err)
	}
	if string(raw) != "{}" {
		t.Fatalf("user_state = %q, want {}", raw)
	}
	// Logs emitted before the loop should still be present.
	logs, err := h.scripts.GetLogs(context.Background(), rid)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) == 0 || logs[0].Message != "pre-loop" {
		t.Fatalf("expected 'pre-loop' log, got %v", logs)
	}
}

func TestRunner_LastRunAtUpdatedOnError(t *testing.T) {
	h := newHarness(t)
	sid := h.createScript(`throw new Error("bad");`)
	rid := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sc, err := h.scripts.Get(context.Background(), sid)
	if err != nil {
		t.Fatalf("Get script: %v", err)
	}
	if sc.LastRunAt == nil {
		t.Fatalf("LastRunAt is nil after error run, want non-nil")
	}
}

func TestRunner_NoSetTimeoutOrFetch(t *testing.T) {
	h := newHarness(t)
	// typeof returns "undefined" for missing globals; this script logs the
	// type so we can confirm the sandbox doesn't leak them.
	sid := h.createScript(`
        ctx.log(typeof setTimeout);
        ctx.log(typeof setInterval);
        ctx.log(typeof fetch);
        ctx.log(typeof process);
    `)
	rid := h.startRun(sid, script.TriggerManual)
	if err := h.runner.Run(context.Background(), sid, rid, script.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}
	logs, err := h.scripts.GetLogs(context.Background(), rid)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 4 {
		t.Fatalf("got %d logs, want 4: %v", len(logs), logs)
	}
	for i, l := range logs {
		if l.Message != "undefined" {
			t.Fatalf("log[%d] = %q, want undefined", i, l.Message)
		}
	}
}
