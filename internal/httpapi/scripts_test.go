package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/srliao/tt/internal/httpapi"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/task"
)

// fxTaskInputWithScript is a tiny helper used by the spawned-task list test
// to build a task.CreateInput with an optional script id.
func fxTaskInputWithScript(title string, sid *int64) task.CreateInput {
	return task.CreateInput{Title: title, SpawnedByScriptID: sid}
}

func decodeScript(t *testing.T, resp *http.Response) script.Script {
	t.Helper()
	var out script.Script
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode script: %v", err)
	}
	return out
}

func TestScripts_CRUD(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/scripts", map[string]any{
		"name":    "daily-foo",
		"code":    "console.log('hi')",
		"enabled": true,
		"schedule": map[string]any{
			"kind": "daily",
		},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status = %d body = %s", resp.StatusCode, string(body))
	}
	created := decodeScript(t, resp)
	if created.Schedule.Kind != script.KindDaily || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}

	// Get
	resp2 := doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/scripts/%d", fx.server.URL, created.ID), nil)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", resp2.StatusCode)
	}

	// List
	resp3 := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/scripts", nil)
	defer func() { _ = resp3.Body.Close() }()
	var listed []script.Script
	if err := json.NewDecoder(resp3.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("list len = %d", len(listed))
	}

	// Patch
	resp4 := doJSON(t, http.MethodPatch, fmt.Sprintf("%s/api/v1/scripts/%d", fx.server.URL, created.ID), map[string]any{
		"name":    "daily-foo",
		"code":    "console.log('updated')",
		"enabled": false,
		"schedule": map[string]any{
			"kind":    "weekly",
			"weekday": "monday",
		},
	})
	defer func() { _ = resp4.Body.Close() }()
	if resp4.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp4.Body)
		t.Fatalf("patch status = %d body = %s", resp4.StatusCode, string(body))
	}
	patched := decodeScript(t, resp4)
	if patched.Schedule.Kind != script.KindWeekly || patched.Schedule.Weekday != script.Monday {
		t.Fatalf("patched schedule = %+v", patched.Schedule)
	}
	if patched.Enabled {
		t.Fatalf("expected disabled after patch")
	}

	// Delete
	resp5 := doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/scripts/%d", fx.server.URL, created.ID), nil)
	defer func() { _ = resp5.Body.Close() }()
	if resp5.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp5.StatusCode)
	}
}

func TestScripts_BadWeekdayRejected(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/scripts", map[string]any{
		"name":    "weird",
		"code":    "",
		"enabled": true,
		"schedule": map[string]any{
			"kind":    "weekly",
			"weekday": "fundayday",
		},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Error.Code != "validation_failed" {
		t.Fatalf("code = %q", env.Error.Code)
	}
}

func TestScripts_MonthlyDayLast(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/scripts", map[string]any{
		"name":    "month",
		"code":    "",
		"enabled": true,
		"schedule": map[string]any{
			"kind": "monthly",
			"day":  "last",
		},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	got := decodeScript(t, resp)
	if got.Schedule.Kind != script.KindMonthly || !got.Schedule.Day.IsLast {
		t.Fatalf("schedule = %+v", got.Schedule)
	}
}

func TestScripts_ManualRunDisabledIs409(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	sc, err := fx.scripts.Create(ctx, script.CreateInput{
		Name:     "disabled",
		Enabled:  false,
		Schedule: script.Schedule{Kind: script.KindDaily},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/scripts/%d/run", fx.server.URL, sc.ID), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
}

func TestScripts_ManualRunHappyPath(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	sc, err := fx.scripts.Create(ctx, script.CreateInput{
		Name:     "good",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/scripts/%d/run", fx.server.URL, sc.ID), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	var body struct {
		RunID int64 `json:"run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.RunID <= 0 {
		t.Fatalf("run_id = %d", body.RunID)
	}
	// The fixture's noopEnqueuer should have observed the call.
	if fx.enq.lastScript != sc.ID || fx.enq.lastRun != body.RunID {
		t.Fatalf("enqueuer not called: %+v", fx.enq)
	}
	// Verify the run row exists.
	run, err := fx.scripts.GetRun(ctx, body.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != script.RunStatusRunning {
		t.Fatalf("status = %q, want running", run.Status)
	}
}

func TestScripts_ManualRunBusyIs503AndFinishesRun(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	fx.enq.err = errors.New("scheduler busy")
	_ = httpapi.ErrSchedulerBusy // ensure package symbol stays referenced from tests

	ctx := context.Background()
	sc, err := fx.scripts.Create(ctx, script.CreateInput{
		Name:     "busy",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/scripts/%d/run", fx.server.URL, sc.ID), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}

	// The handler must have marked the just-created run as errored to keep
	// the row from sitting in 'running'.
	runs, err := fx.scripts.ListRunsByScript(ctx, sc.ID, 5, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs len = %d", len(runs))
	}
	if runs[0].Status != script.RunStatusError {
		t.Fatalf("run status = %q, want error", runs[0].Status)
	}
}

func TestScripts_ListRunsByScript(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	sc, _ := fx.scripts.Create(ctx, script.CreateInput{
		Name:     "x",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	for i := 0; i < 3; i++ {
		_, _ = fx.scripts.StartRun(ctx, sc.ID, script.TriggerScheduled)
	}

	resp := doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/scripts/%d/runs", fx.server.URL, sc.ID), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var runs []script.Run
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("runs len = %d", len(runs))
	}
}

func TestScripts_ListTasksByScript(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	sc, _ := fx.scripts.Create(ctx, script.CreateInput{
		Name:     "spawner",
		Enabled:  true,
		Schedule: script.Schedule{Kind: script.KindEveryTick},
	})
	sid := sc.ID
	// Two tasks spawned by this script + one unrelated.
	if _, err := fx.tasks.Create(ctx, fxTaskInputWithScript("a", &sid)); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := fx.tasks.Create(ctx, fxTaskInputWithScript("b", &sid)); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if _, err := fx.tasks.Create(ctx, fxTaskInputWithScript("c", nil)); err != nil {
		t.Fatalf("create c: %v", err)
	}

	resp := doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/scripts/%d/tasks", fx.server.URL, sc.ID), nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	tasks := decodeTasks(t, resp)
	if len(tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2", len(tasks))
	}
}
