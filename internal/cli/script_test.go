package cli_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/srliao/tt/internal/script"
)

func writeCode(t *testing.T, code string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.js")
	if err := os.WriteFile(p, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

type scriptOut struct {
	script.Script
	NextStep string `json:"next_step"`
}

func TestScriptCheck_IsLocal(t *testing.T) {
	f := newFixture(t)
	f.env = map[string]string{} // no server configured

	var got map[string]any
	f.runStdin(`ctx.log.info("hi")`, "script", "check", "--schedule", "monthly:last").ok(t, &got)
	if got["ok"] != true {
		t.Fatalf("got %v", got)
	}
	sch, _ := got["schedule"].(map[string]any)
	if sch["kind"] != "monthly" || sch["day"] != "last" {
		t.Fatalf("schedule = %v", got["schedule"])
	}

	e := f.runStdin("ctx.log.info(", "script", "check", "--schedule", "daily").fails(t, 3, "validation_failed")
	if e.Details["line"] == nil {
		t.Fatalf("details should carry the position: %v", e.Details)
	}
	f.runStdin("1", "script", "check", "--schedule", "weekly:someday").fails(t, 3, "validation_failed")
	f.runStdin("1", "script", "check", "--schedule", "monthly:32").fails(t, 3, "validation_failed")
}

func TestScriptCreate_SavedDisabledWithNextStep(t *testing.T) {
	f := newFixture(t)
	code := writeCode(t, `ctx.queueTask({title: "Water plants"})`)

	var got scriptOut
	f.run("script", "create", "--name", "plants", "--code", code, "--schedule", "weekly:monday").ok(t, &got)

	if got.ID == 0 || got.Enabled || got.NextStep != "enable in UI" {
		t.Fatalf("got %+v", got)
	}
	if got.Schedule.Kind != script.KindWeekly || got.Schedule.Weekday != script.Monday {
		t.Fatalf("schedule = %+v", got.Schedule)
	}
}

func TestScriptCreate_CompileErrorNeverReachesServer(t *testing.T) {
	f := newFixture(t)
	f.runStdin("function (", "script", "create", "--name", "bad", "--schedule", "daily").fails(t, 3, "validation_failed")
	if n := len(f.headers); n != 0 {
		t.Fatalf("%d requests reached the server", n)
	}
}

// Enabling happens in the UI; an update from the CLI keeps whatever the
// user chose.
func TestScriptUpdate_KeepsEnabledAndUnsetFields(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc, err := f.scripts.Create(ctx, script.CreateInput{
		Name: "plants", Code: "1", Enabled: true, Schedule: script.Schedule{Kind: script.KindDaily},
	})
	if err != nil {
		t.Fatal(err)
	}

	var got scriptOut
	f.runStdin("2", "script", "update", id(sc.ID), "--code", "-").ok(t, &got)

	if !got.Enabled || got.Code != "2" || got.Name != "plants" || got.Schedule.Kind != script.KindDaily {
		t.Fatalf("got %+v", got)
	}
}

// A UI toggle that lands between the CLI reading the script and saving it
// survives, because update never sends enabled.
func TestScriptUpdate_DoesNotUndoUIToggle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	in := script.CreateInput{Name: "plants", Code: "1", Enabled: true, Schedule: script.Schedule{Kind: script.KindDaily}}
	sc, err := f.scripts.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	f.gate = func(_ http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			off := in
			off.Enabled = false
			if _, err := f.scripts.Update(ctx, sc.ID, off); err != nil {
				t.Error(err)
			}
		}
	}

	var got scriptOut
	f.runStdin("2", "script", "update", id(sc.ID), "--code", "-").ok(t, &got)

	if got.Enabled || got.Code != "2" || got.NextStep != "enable in UI" {
		t.Fatalf("got %+v, want the UI's disable kept", got)
	}
}

func TestScriptListGetRunRuns(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc, err := f.scripts.Create(ctx, script.CreateInput{
		Name: "plants", Code: "1", Schedule: script.Schedule{Kind: script.KindDaily},
	})
	if err != nil {
		t.Fatal(err)
	}

	var list struct {
		Scripts []script.Script `json:"scripts"`
	}
	f.run("script", "list").ok(t, &list)
	if len(list.Scripts) != 1 || list.Scripts[0].ID != sc.ID {
		t.Fatalf("list = %+v", list)
	}
	var got script.Script
	f.run("script", "get", id(sc.ID)).ok(t, &got)
	if got.Name != "plants" {
		t.Fatalf("get = %+v", got)
	}

	// Saved disabled, so a manual run is refused until enabled in the UI.
	f.run("script", "run", id(sc.ID)).fails(t, 3, "validation_failed")

	run, err := f.scripts.StartRun(ctx, sc.ID, script.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	var runs struct {
		Runs []script.Run `json:"runs"`
	}
	f.run("script", "runs", id(sc.ID)).ok(t, &runs)
	if len(runs.Runs) != 1 || runs.Runs[0].ID != run.ID {
		t.Fatalf("runs = %+v", runs)
	}
}
