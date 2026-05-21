package script_test

import (
	"context"
	"testing"
	"time"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/script"
)

// newSvc spins up an in-memory store and wires a script service around it.
func newSvc(t *testing.T) *script.Impl {
	t.Helper()
	store := dbtest.New(t)
	return script.New(store)
}

func TestCreateAndGetRoundTripWeekly(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	sc, err := svc.Create(ctx, script.CreateInput{
		Name:    "weekly check",
		Code:    "print('hi')",
		Enabled: true,
		Schedule: script.Schedule{
			Kind:    script.KindWeekly,
			Weekday: script.Wednesday,
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sc.ID == 0 {
		t.Errorf("expected non-zero id, got %d", sc.ID)
	}

	got, err := svc.Get(ctx, sc.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "weekly check" {
		t.Errorf("Name = %q, want %q", got.Name, "weekly check")
	}
	if got.Code != "print('hi')" {
		t.Errorf("Code mismatch: %q", got.Code)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if got.Schedule.Kind != script.KindWeekly {
		t.Errorf("Schedule.Kind = %q, want weekly", got.Schedule.Kind)
	}
	if got.Schedule.Weekday != script.Wednesday {
		t.Errorf("Schedule.Weekday = %q, want wednesday", got.Schedule.Weekday)
	}
}

func TestCreateTrimsAndRejectsEmptyName(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	sc, err := svc.Create(ctx, script.CreateInput{
		Name:     "  spaced  ",
		Schedule: script.Schedule{Kind: script.KindDaily},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sc.Name != "spaced" {
		t.Errorf("Name = %q, want %q", sc.Name, "spaced")
	}

	if _, err := svc.Create(ctx, script.CreateInput{
		Name:     "   ",
		Schedule: script.Schedule{Kind: script.KindDaily},
	}); err == nil {
		t.Fatalf("expected error for empty name")
	}
}

func TestUpdateReplacesFieldsAndSwitchesKind(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	sc, err := svc.Create(ctx, script.CreateInput{
		Name:     "orig",
		Code:     "a",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindDaily},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.Update(ctx, sc.ID, script.UpdateInput{
		Name:    "renamed",
		Code:    "b",
		Enabled: false,
		Schedule: script.Schedule{
			Kind: script.KindMonthly,
			Day:  script.MonthlyDay{IsLast: true, Valid: true},
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "renamed" || updated.Code != "b" || updated.Enabled {
		t.Errorf("Update did not replace fields: %+v", updated)
	}
	if updated.Schedule.Kind != script.KindMonthly || !updated.Schedule.Day.IsLast {
		t.Errorf("Update did not switch schedule kind: %+v", updated.Schedule)
	}

	if err := svc.Delete(ctx, sc.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDueAtFiltersEnabledAndMatching(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	// Enabled weekly Monday — should match on a Monday.
	mon, err := svc.Create(ctx, script.CreateInput{
		Name:     "weekly mon",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindWeekly, Weekday: script.Monday},
	})
	if err != nil {
		t.Fatalf("Create weekly mon: %v", err)
	}

	// Enabled weekly Friday — should NOT match on a Monday.
	if _, err := svc.Create(ctx, script.CreateInput{
		Name:     "weekly fri",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindWeekly, Weekday: script.Friday},
	}); err != nil {
		t.Fatalf("Create weekly fri: %v", err)
	}

	// Disabled every-tick — should be excluded even though it would match.
	if _, err := svc.Create(ctx, script.CreateInput{
		Name:     "disabled every",
		Enabled:  false,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	}); err != nil {
		t.Fatalf("Create disabled: %v", err)
	}

	monday := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC) // Monday
	due, err := svc.DueAt(ctx, monday)
	if err != nil {
		t.Fatalf("DueAt: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("DueAt = %d scripts, want 1: %+v", len(due), due)
	}
	if due[0].ID != mon.ID {
		t.Errorf("DueAt id = %d, want %d", due[0].ID, mon.ID)
	}
}

func TestRunLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	sc := mustCreate(t, ctx, svc)

	run, err := svc.StartRun(ctx, sc.ID, script.TriggerManual)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != script.RunStatusRunning {
		t.Errorf("StartRun status = %q, want running", run.Status)
	}
	if run.StartedAt.IsZero() {
		t.Errorf("StartedAt is zero")
	}

	if err := svc.AppendLog(ctx, run.ID, script.LogInfo, "hello"); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}
	logs, err := svc.GetLogs(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("GetLogs len = %d, want 1", len(logs))
	}
	if logs[0].Message != "hello" || logs[0].Level != script.LogInfo {
		t.Errorf("log = %+v, want {info hello}", logs[0])
	}

	if err := svc.FinishRun(ctx, run.ID, script.RunStatusOK, "", []int64{42}); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != script.RunStatusOK {
		t.Errorf("GetRun status = %q, want ok", got.Status)
	}
	if len(got.SpawnedTaskIDs) != 1 || got.SpawnedTaskIDs[0] != 42 {
		t.Errorf("SpawnedTaskIDs = %v, want [42]", got.SpawnedTaskIDs)
	}
	if got.FinishedAt == nil {
		t.Errorf("FinishedAt is nil")
	}
}

func TestRecoverOrphanedRuns(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	sc := mustCreate(t, ctx, svc)
	run, err := svc.StartRun(ctx, sc.ID, script.TriggerScheduled)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	if err := svc.RecoverOrphanedRuns(ctx); err != nil {
		t.Fatalf("RecoverOrphanedRuns: %v", err)
	}

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != script.RunStatusError {
		t.Errorf("Status = %q, want error", got.Status)
	}
	if got.ErrorMessage != "interrupted (binary restart)" {
		t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, "interrupted (binary restart)")
	}
}

func TestUserStateReadWrite(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	sc := mustCreate(t, ctx, svc)

	payload := []byte(`{"k":1}`)
	if err := svc.WriteUserState(ctx, sc.ID, payload); err != nil {
		t.Fatalf("WriteUserState: %v", err)
	}
	got, err := svc.ReadUserState(ctx, sc.ID)
	if err != nil {
		t.Fatalf("ReadUserState: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("ReadUserState = %q, want %q", string(got), string(payload))
	}
}

func TestPruneRunsKeeps500(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	sc := mustCreate(t, ctx, svc)
	for i := 0; i < 510; i++ {
		run, err := svc.StartRun(ctx, sc.ID, script.TriggerScheduled)
		if err != nil {
			t.Fatalf("StartRun #%d: %v", i, err)
		}
		if err := svc.FinishRun(ctx, run.ID, script.RunStatusOK, "", nil); err != nil {
			t.Fatalf("FinishRun #%d: %v", i, err)
		}
	}

	if err := svc.PruneRuns(ctx, 500); err != nil {
		t.Fatalf("PruneRuns: %v", err)
	}

	count, err := svc.CountRuns(ctx)
	if err != nil {
		t.Fatalf("CountRuns: %v", err)
	}
	if count != 500 {
		t.Errorf("CountRuns = %d, want 500", count)
	}
}

func TestSetLastRunAtUpdatesScript(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	sc := mustCreate(t, ctx, svc)

	ts := time.Date(2026, 5, 21, 10, 30, 0, 0, time.UTC)
	if err := svc.SetLastRunAt(ctx, sc.ID, ts); err != nil {
		t.Fatalf("SetLastRunAt: %v", err)
	}
	got, err := svc.Get(ctx, sc.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastRunAt == nil {
		t.Fatalf("LastRunAt is nil")
	}
	if !got.LastRunAt.Equal(ts) {
		t.Errorf("LastRunAt = %s, want %s", got.LastRunAt, ts)
	}
}

func TestListReturnsAll(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	mustCreate(t, ctx, svc)
	mustCreate(t, ctx, svc)

	all, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List len = %d, want 2", len(all))
	}
}

// mustCreate is a small helper for tests that need any valid script row but
// don't care about its specific fields.
func mustCreate(t *testing.T, ctx context.Context, svc *script.Impl) script.Script {
	t.Helper()
	sc, err := svc.Create(ctx, script.CreateInput{
		Name:     "test-script",
		Code:     "noop",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return sc
}
