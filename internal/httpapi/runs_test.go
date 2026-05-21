package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/srliao/tt/internal/script"
)

func TestRuns_List(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	scA, _ := fx.scripts.Create(ctx, script.CreateInput{Name: "a", Enabled: true, Schedule: script.Schedule{Kind: script.KindEveryTick}})
	scB, _ := fx.scripts.Create(ctx, script.CreateInput{Name: "b", Enabled: true, Schedule: script.Schedule{Kind: script.KindEveryTick}})

	for i := 0; i < 2; i++ {
		_, _ = fx.scripts.StartRun(ctx, scA.ID, script.TriggerScheduled)
	}
	_, _ = fx.scripts.StartRun(ctx, scB.ID, script.TriggerScheduled)

	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/runs", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var runs []script.Run
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("len = %d, want 3", len(runs))
	}

	// Filter by script_id
	u, _ := url.Parse(fx.server.URL + "/api/v1/runs")
	u.RawQuery = fmt.Sprintf("script_id=%d", scA.ID)
	resp2 := doJSON(t, http.MethodGet, u.String(), nil)
	defer resp2.Body.Close()
	var only []script.Run
	if err := json.NewDecoder(resp2.Body).Decode(&only); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(only) != 2 {
		t.Fatalf("len = %d, want 2", len(only))
	}
}

func TestRuns_FilterByStatus(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	sc, _ := fx.scripts.Create(ctx, script.CreateInput{Name: "s", Enabled: true, Schedule: script.Schedule{Kind: script.KindEveryTick}})
	r1, _ := fx.scripts.StartRun(ctx, sc.ID, script.TriggerScheduled)
	r2, _ := fx.scripts.StartRun(ctx, sc.ID, script.TriggerScheduled)
	_ = fx.scripts.FinishRun(ctx, r1.ID, script.RunStatusOK, "", nil)
	_ = fx.scripts.FinishRun(ctx, r2.ID, script.RunStatusError, "boom", nil)

	u, _ := url.Parse(fx.server.URL + "/api/v1/runs")
	u.RawQuery = "status=ok"
	resp := doJSON(t, http.MethodGet, u.String(), nil)
	defer resp.Body.Close()
	var runs []script.Run
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != script.RunStatusOK {
		t.Fatalf("filtered = %+v", runs)
	}
}

func TestRuns_GetDetail(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	sc, _ := fx.scripts.Create(ctx, script.CreateInput{Name: "s", Enabled: true, Schedule: script.Schedule{Kind: script.KindEveryTick}})
	run, _ := fx.scripts.StartRun(ctx, sc.ID, script.TriggerScheduled)
	_ = fx.scripts.AppendLog(ctx, run.ID, script.LogInfo, "hello")
	_ = fx.scripts.AppendLog(ctx, run.ID, script.LogWarn, "ish")
	_ = fx.scripts.FinishRun(ctx, run.ID, script.RunStatusOK, "", nil)

	resp := doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/runs/%d", fx.server.URL, run.ID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	var body struct {
		ID            int64                   `json:"id"`
		Status        script.RunStatus        `json:"status"`
		Logs          []script.Log            `json:"logs"`
		SpawnedTasks  []map[string]any        `json:"spawned_tasks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != run.ID {
		t.Fatalf("id = %d", body.ID)
	}
	if body.Status != script.RunStatusOK {
		t.Fatalf("status = %q", body.Status)
	}
	if len(body.Logs) != 2 {
		t.Fatalf("logs len = %d, want 2", len(body.Logs))
	}
	if body.SpawnedTasks == nil {
		t.Fatalf("spawned_tasks is nil; expected empty array")
	}
}

func TestRuns_GetDetailNotFound(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/runs/99999", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
