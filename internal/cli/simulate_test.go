package cli_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/task"
)

type simRun struct {
	At         time.Time      `json:"at"`
	Trigger    string         `json:"trigger"`
	Spawned    []task.Task    `json:"spawned"`
	Logs       []script.Log   `json:"logs"`
	StateAfter map[string]any `json:"state_after"`
	Error      *string        `json:"error"`
}

type simOut struct {
	Runs    []simRun `json:"runs"`
	Summary struct {
		SpawnDates []string `json:"spawn_dates"`
	} `json:"summary"`
}

// simFixture pins the CLI clock to Thursday 2026-11-05 07:00 in the
// fixture server's zone (America/New_York).
func simFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	f.now = func() time.Time { return now }
	return f
}

func (f *fixture) storedScript(sch script.Schedule, code, state string) script.Script {
	f.t.Helper()
	ctx := context.Background()
	sc, err := f.scripts.Create(ctx, script.CreateInput{Name: "s", Code: code, Schedule: sch})
	if err != nil {
		f.t.Fatal(err)
	}
	if state != "" {
		if err := f.scripts.WriteUserState(ctx, sc.ID, []byte(state)); err != nil {
			f.t.Fatal(err)
		}
	}
	return sc
}

// pastSpawn records an ok run of scriptID at at that spawned one task, which
// is still open.
func (f *fixture) pastSpawn(scriptID int64, at time.Time) {
	f.t.Helper()
	ctx := context.Background()
	clock := func() time.Time { return at }
	scripts := script.New(f.store, script.WithClock(clock))
	run, err := scripts.StartRun(ctx, scriptID, script.TriggerScheduled)
	if err != nil {
		f.t.Fatal(err)
	}
	spawn, err := task.New(f.store, task.WithClock(clock)).Create(ctx,
		task.CreateInput{Title: "Water plants", SpawnedByScriptID: &scriptID})
	if err != nil {
		f.t.Fatal(err)
	}
	if err := scripts.FinishRun(ctx, run.ID, script.RunStatusOK, "", []int64{spawn.ID}); err != nil {
		f.t.Fatal(err)
	}
}

func TestSimulate_WeeklyFiresOnEachMondayOfDefaultRange(t *testing.T) {
	f := simFixture(t)
	code := writeCode(t, `ctx.queueTask({title: "Take out bins"})`)

	var got simOut
	f.run("script", "simulate", "--code", code, "--schedule", "weekly:monday").ok(t, &got)

	want := []string{"2026-11-09", "2026-11-16", "2026-11-23", "2026-11-30", "2026-12-07"}
	if !slices.Equal(got.Summary.SpawnDates, want) {
		t.Fatalf("spawn_dates = %v, want %v", got.Summary.SpawnDates, want)
	}
	if len(got.Runs) != 5 {
		t.Fatalf("runs = %d, want 5", len(got.Runs))
	}
	r := got.Runs[0]
	if r.Trigger != "scheduled" || len(r.Spawned) != 1 || r.Spawned[0].Title != "Take out bins" || r.Error != nil {
		t.Fatalf("first run = %+v", r)
	}
	// Midnight Monday in New York.
	if want := time.Date(2026, 11, 9, 5, 0, 0, 0, time.UTC); !r.At.Equal(want) {
		t.Fatalf("first run at %s, want %s", r.At, want)
	}
	if !r.Spawned[0].CreatedAt.Equal(r.At) {
		t.Fatalf("spawn created_at %s should follow the fake clock (%s)", r.Spawned[0].CreatedAt, r.At)
	}
}

// Spawns again 3 days after the last spawn was completed.
const afterCompletedCode = `
const last = ctx.lastSpawn;
if (!last || (last.completed_at && ctx.daysSince(last.completed_at) >= 3)) {
  ctx.queueTask({title: "Water plants"});
}`

func TestSimulate_DaysAfterCompletedRepeatsOnlyWhenSpawnsComplete(t *testing.T) {
	f := simFixture(t)
	code := writeCode(t, afterCompletedCode)

	var completing simOut
	f.run("script", "simulate", "--code", code, "--schedule", "daily", "--complete-after", "0d").ok(t, &completing)
	want := []string{
		"2026-11-05", "2026-11-08", "2026-11-11", "2026-11-14", "2026-11-17", "2026-11-20",
		"2026-11-23", "2026-11-26", "2026-11-29", "2026-12-02", "2026-12-05", "2026-12-08",
	}
	if !slices.Equal(completing.Summary.SpawnDates, want) {
		t.Fatalf("with --complete-after 0d: spawn_dates = %v, want %v", completing.Summary.SpawnDates, want)
	}

	var open simOut
	f.run("script", "simulate", "--code", code, "--schedule", "daily").ok(t, &open)
	if !slices.Equal(open.Summary.SpawnDates, []string{"2026-11-05"}) {
		t.Fatalf("without --complete-after: spawn_dates = %v", open.Summary.SpawnDates)
	}
	// Daily: one run per day in range, spawning or not.
	if len(open.Runs) != 36 {
		t.Fatalf("runs = %d, want 36", len(open.Runs))
	}
}

// --id uses the stored script's state, even though it is disabled, and the
// state carries from run to run.
func TestSimulate_StoredDisabledScriptCarriesState(t *testing.T) {
	f := simFixture(t)
	sc := f.storedScript(script.Schedule{Kind: script.KindDaily},
		`const n = ctx.state.get("n") + 1; ctx.state.set("n", n); ctx.log.info("run " + n);`, `{"n": 5}`)
	if sc.Enabled {
		t.Fatal("script should be disabled")
	}

	var got simOut
	f.run("script", "simulate", "--id", id(sc.ID), "--from", "2026-11-05", "--to", "2026-11-07").ok(t, &got)

	if len(got.Runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(got.Runs))
	}
	for i, r := range got.Runs {
		n := float64(6 + i)
		if r.StateAfter["n"] != n {
			t.Errorf("run %d state_after = %v, want n=%v", i, r.StateAfter, n)
		}
		if len(r.Logs) != 1 || r.Logs[0].Message != "run "+id(int64(n)) {
			t.Errorf("run %d logs = %+v", i, r.Logs)
		}
	}
	if want := time.Date(2026, 11, 5, 5, 0, 0, 0, time.UTC); !got.Runs[0].At.Equal(want) {
		t.Fatalf("first run at %s, want %s (midnight in the snapshot zone)", got.Runs[0].At, want)
	}

	// Nothing ran on the server.
	runs, err := f.scripts.ListRunsByScript(context.Background(), sc.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("server has %d runs", len(runs))
	}
}

// With --id, spawns already open in the snapshot are completed too, so a
// script waiting on its last spawn can fire again.
func TestSimulate_CompleteAfterCoversSpawnsAlreadyOpen(t *testing.T) {
	f := simFixture(t)
	sc := f.storedScript(script.Schedule{Kind: script.KindDaily}, afterCompletedCode, "")
	f.pastSpawn(sc.ID, time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC))

	var got simOut
	f.run("script", "simulate", "--id", id(sc.ID), "--from", "2026-11-05", "--to", "2026-11-08",
		"--complete-after", "0d").ok(t, &got)

	if want := []string{"2026-11-05", "2026-11-08"}; !slices.Equal(got.Summary.SpawnDates, want) {
		t.Fatalf("spawn_dates = %v, want %v", got.Summary.SpawnDates, want)
	}
}

func TestSimulate_FromAloneRunsThirtyFiveDays(t *testing.T) {
	f := simFixture(t)
	code := writeCode(t, `ctx.queueTask({title: "x"})`)

	var got simOut
	f.run("script", "simulate", "--code", code, "--schedule", "daily", "--from", "2027-01-01").ok(t, &got)

	dates := got.Summary.SpawnDates
	if len(dates) != 36 || dates[0] != "2027-01-01" || dates[35] != "2027-02-05" {
		t.Fatalf("spawn_dates = %v, want 2027-01-01 through 2027-02-05", dates)
	}
}

func TestSimulate_PreviewEditKeepsStoredState(t *testing.T) {
	f := simFixture(t)
	sc := f.storedScript(script.Schedule{Kind: script.KindDaily}, `1`, `{"n": 5}`)
	code := writeCode(t, `ctx.state.set("n", ctx.state.get("n") * 10)`)

	var got simOut
	f.run("script", "simulate", "--id", id(sc.ID), "--code", code, "--schedule", "weekly:friday",
		"--from", "2026-11-05", "--to", "2026-11-13").ok(t, &got)

	if len(got.Runs) != 2 || got.Runs[0].StateAfter["n"] != float64(50) || got.Runs[1].StateAfter["n"] != float64(500) {
		t.Fatalf("runs = %+v", got.Runs)
	}
}

func TestSimulate_ScriptErrorIsReportedPerRun(t *testing.T) {
	f := simFixture(t)
	code := writeCode(t, `throw new Error("boom")`)

	var got simOut
	f.run("script", "simulate", "--code", code, "--schedule", "daily", "--from", "2026-11-05", "--to", "2026-11-05").ok(t, &got)

	if len(got.Runs) != 1 || got.Runs[0].Error == nil {
		t.Fatalf("runs = %+v", got.Runs)
	}
}

func TestSimulate_UsageErrors(t *testing.T) {
	f := simFixture(t)
	code := writeCode(t, `1`)
	f.run("script", "simulate", "--code", code).fails(t, 1, "usage")
	f.run("script", "simulate").fails(t, 1, "usage")
	f.run("script", "simulate", "--code", code, "--schedule", "daily", "--complete-after", "soon").fails(t, 1, "usage")
	f.run("script", "simulate", "--code", code, "--schedule", "daily", "--from", "next week").fails(t, 3, "validation_failed")
	f.run("script", "simulate", "--code", code, "--schedule", "daily", "--tz", "Mars/Olympus").fails(t, 1, "usage")
	f.run("script", "simulate", "--id", "999").fails(t, 2, "not_found")
}
