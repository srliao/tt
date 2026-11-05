package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/srliao/tt/internal/task"
)

func TestPlans_ApplyReturnsIDsAndTasks(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	existing := createTasks(t, fx, "existing")[0]

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/plans", map[string]any{
		"tasks": []map[string]any{
			{"ref": "design", "title": "Design", "stage": true},
			{"ref": "impl", "title": "Implement"},
		},
		"deps": []map[string]any{
			{"task": "impl", "depends_on": "design"},
			{"task": "impl", "depends_on": existing},
		},
	})
	defer func() { _ = resp.Body.Close() }()
	expectOK(t, resp)
	var out task.PlanResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	design, impl := out.IDs["design"], out.IDs["impl"]
	if len(out.Tasks) != 3 || out.Tasks[0].ID != design || out.Tasks[1].ID != impl || out.Tasks[2].ID != existing {
		t.Fatalf("tasks = %+v, ids = %v", out.Tasks, out.IDs)
	}
	got := out.Tasks[1]
	if !got.Blocked || !slices.Contains(got.BlockedBy, design) || !slices.Contains(got.BlockedBy, existing) {
		t.Fatalf("impl = %+v", got)
	}
}

func TestPlans_ApplyErrors(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	if _, err := fx.tags.Create(context.Background(), "work"); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	url := fx.server.URL + "/api/v1/plans"

	details := expectError(t, doJSON(t, http.MethodPost, url, map[string]any{
		"tasks": []map[string]any{{"ref": "a", "title": "A", "tags": []string{"nope"}}},
	}), http.StatusBadRequest, "validation_failed")
	if details["field"] != "tags" || details["ref"] != "a" {
		t.Fatalf("details = %v", details)
	}
	if got := details["existing_tags"]; len(got.([]any)) != 1 {
		t.Fatalf("existing_tags = %v", got)
	}

	details = expectError(t, doJSON(t, http.MethodPost, url, map[string]any{
		"tasks": []map[string]any{{"ref": "a", "title": "A"}},
		"deps":  []map[string]any{{"task": "a", "depends_on": 999}},
	}), http.StatusNotFound, "not_found")
	if details["op"] != "add" || details["index"] != float64(0) {
		t.Fatalf("details = %v", details)
	}

	details = expectError(t, doJSON(t, http.MethodPost, url, map[string]any{
		"tasks": []map[string]any{{"ref": "a", "title": "A"}, {"ref": "b", "title": "B"}},
		"deps": []map[string]any{
			{"task": "a", "depends_on": "b"},
			{"task": "b", "depends_on": "a"},
		},
	}), http.StatusConflict, "conflict")
	if details["rule"] != task.RuleCycle || details["index"] != float64(1) {
		t.Fatalf("details = %v", details)
	}

	expectError(t, doJSON(t, http.MethodPost, url, map[string]any{
		"deps": []map[string]any{{"task": true, "depends_on": 1}},
	}), http.StatusBadRequest, "validation_failed")

	all, err := fx.tasks.List(context.Background(), task.FilterSort{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("failed plans left %d tasks behind", len(all))
	}
}
