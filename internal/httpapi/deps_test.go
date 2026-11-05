package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/srliao/tt/internal/task"
)

type errorEnv struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

func expectError(t *testing.T, resp *http.Response, status int, code string) map[string]any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != status {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, status, string(body))
	}
	var env errorEnv
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Error.Code != code {
		t.Fatalf("code = %q, want %q", env.Error.Code, code)
	}
	return env.Error.Details
}

func expectOK(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
}

func createTasks(t *testing.T, fx *serverFixture, titles ...string) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(titles))
	for _, title := range titles {
		created, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: title})
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		ids = append(ids, created.ID)
	}
	return ids
}

func addDep(t *testing.T, fx *serverFixture, taskID, dependsOnID int64) *http.Response {
	t.Helper()
	return doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/tasks/%d/deps", fx.server.URL, taskID),
		map[string]any{"depends_on_id": dependsOnID})
}

func idList(t *testing.T, v any) []int64 {
	t.Helper()
	raw, ok := v.([]any)
	if !ok {
		t.Fatalf("not a list: %#v", v)
	}
	out := make([]int64, 0, len(raw))
	for _, n := range raw {
		out = append(out, int64(n.(float64)))
	}
	return out
}

func TestDeps_AddReturnsTaskWithEdges(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ids := createTasks(t, fx, "a", "b")

	resp := addDep(t, fx, ids[0], ids[1])
	defer func() { _ = resp.Body.Close() }()
	expectOK(t, resp)
	got := decodeTask(t, resp)
	if !slices.Equal(got.BlockedBy, []int64{ids[1]}) || !got.Blocked {
		t.Fatalf("blocked_by = %v, blocked = %v", got.BlockedBy, got.Blocked)
	}

	resp2 := addDep(t, fx, ids[0], ids[1])
	defer func() { _ = resp2.Body.Close() }()
	expectOK(t, resp2)

	resp3 := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/deps", nil)
	defer func() { _ = resp3.Body.Close() }()
	var edges [][]int64
	if err := json.NewDecoder(resp3.Body).Decode(&edges); err != nil {
		t.Fatalf("decode edges: %v", err)
	}
	if len(edges) != 1 || !slices.Equal(edges[0], []int64{ids[0], ids[1]}) {
		t.Fatalf("edges = %v", edges)
	}
}

func TestDeps_RuleViolationsAre409WithDetails(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ids := createTasks(t, fx, "a", "b", "c", "done")
	a, b, c, done := ids[0], ids[1], ids[2], ids[3]
	ctx := context.Background()
	if _, err := fx.tasks.SetState(ctx, done, task.StateDone); err != nil {
		t.Fatalf("set done: %v", err)
	}

	d := expectError(t, addDep(t, fx, done, a), http.StatusConflict, "conflict")
	if d["rule"] != task.RuleBlockedTaskNotOpen {
		t.Fatalf("R1 details = %v", d)
	}

	expectOK(t, addDep(t, fx, a, b))
	expectOK(t, addDep(t, fx, b, c))
	d = expectError(t, addDep(t, fx, c, a), http.StatusConflict, "conflict")
	if d["rule"] != task.RuleCycle || !slices.Equal(idList(t, d["path"]), []int64{c, a, b, c}) {
		t.Fatalf("R2 details = %v", d)
	}

	d = expectError(t, doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/tasks/%d/state", fx.server.URL, a),
		map[string]any{"state": "done"}), http.StatusConflict, "conflict")
	if d["rule"] != task.RuleBlocked || !slices.Equal(idList(t, d["blockers"]), []int64{b}) {
		t.Fatalf("R4 details = %v", d)
	}

	d = expectError(t, doJSON(t, http.MethodDelete, fmt.Sprintf("%s/api/v1/tasks/%d", fx.server.URL, b), nil),
		http.StatusConflict, "conflict")
	if d["rule"] != task.RuleHasOpenDependents || !slices.Equal(idList(t, d["dependents"]), []int64{a}) {
		t.Fatalf("R6 details = %v", d)
	}
}

func TestDeps_UnknownTaskIs404(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	a := createTasks(t, fx, "a")[0]

	expectError(t, addDep(t, fx, a, 999), http.StatusNotFound, "not_found")
	expectError(t, addDep(t, fx, 999, a), http.StatusNotFound, "not_found")
	expectError(t, doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/deps/batch",
		map[string]any{"add": [][]int64{{a, 999}}}), http.StatusNotFound, "not_found")
}

func TestDeps_RemoveIsAlwaysAllowed(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ids := createTasks(t, fx, "a", "b")
	expectOK(t, addDep(t, fx, ids[0], ids[1]))
	if _, err := fx.tasks.SetState(context.Background(), ids[0], task.StateCancelled); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	url := fmt.Sprintf("%s/api/v1/tasks/%d/deps/%d", fx.server.URL, ids[0], ids[1])
	resp := doJSON(t, http.MethodDelete, url, nil)
	defer func() { _ = resp.Body.Close() }()
	expectOK(t, resp)
	if got := decodeTask(t, resp); len(got.BlockedBy) != 0 {
		t.Fatalf("blocked_by = %v", got.BlockedBy)
	}
	resp2 := doJSON(t, http.MethodDelete, url, nil)
	defer func() { _ = resp2.Body.Close() }()
	expectOK(t, resp2)
}

func TestDeps_Batch(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ids := createTasks(t, fx, "a", "b", "c")
	a, b, c := ids[0], ids[1], ids[2]
	batchURL := fx.server.URL + "/api/v1/deps/batch"
	expectOK(t, addDep(t, fx, b, a))

	d := expectError(t, doJSON(t, http.MethodPost, batchURL, map[string]any{
		"add": [][]int64{{c, a}, {a, c}},
	}), http.StatusConflict, "conflict")
	if d["rule"] != task.RuleCycle || d["index"] != float64(1) || !slices.Equal(idList(t, d["pair"]), []int64{a, c}) {
		t.Fatalf("details = %v", d)
	}

	resp := doJSON(t, http.MethodPost, batchURL, map[string]any{
		"remove": [][]int64{{b, a}},
		"add":    [][]int64{{a, b}},
	})
	defer func() { _ = resp.Body.Close() }()
	expectOK(t, resp)
	if got := decodeTasks(t, resp); len(got) != 2 || !slices.Equal(got[0].BlockedBy, []int64{b}) {
		t.Fatalf("batch tasks = %+v", got)
	}

	edges, err := fx.tasks.ListDeps(context.Background())
	if err != nil {
		t.Fatalf("ListDeps: %v", err)
	}
	if want := []task.Edge{{TaskID: a, DependsOnID: b}}; !slices.Equal(edges, want) {
		t.Fatalf("edges = %v, want %v (failed batch must not apply)", edges, want)
	}

	tooMany := make([][]int64, task.MaxDepBatch+1)
	for i := range tooMany {
		tooMany[i] = []int64{a, b}
	}
	expectError(t, doJSON(t, http.MethodPost, batchURL, map[string]any{"add": tooMany}),
		http.StatusBadRequest, "validation_failed")
	expectError(t, doJSON(t, http.MethodPost, batchURL, map[string]any{"add": [][]int64{{a}}}),
		http.StatusBadRequest, "validation_failed")
}

func TestDeps_Graph(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ids := createTasks(t, fx, "a", "b", "c", "d")
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]
	for _, e := range [][2]int64{{a, b}, {a, c}, {b, d}, {c, d}} {
		expectOK(t, addDep(t, fx, e[0], e[1]))
	}

	get := func(id int64, dir string) (task.Graph, *http.Response) {
		resp := doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/tasks/%d/graph%s", fx.server.URL, id, dir), nil)
		var g task.Graph
		if resp.StatusCode == http.StatusOK {
			defer func() { _ = resp.Body.Close() }()
			if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
				t.Fatalf("decode graph: %v", err)
			}
		}
		return g, resp
	}
	ids4 := func(g task.Graph) []int64 {
		var out []int64
		for _, tk := range g.Tasks {
			out = append(out, tk.ID)
		}
		return out
	}

	if g, _ := get(b, ""); !slices.Equal(ids4(g), []int64{a, b, d}) || len(g.Edges) != 2 {
		t.Fatalf("both from b = %v %v", ids4(g), g.Edges)
	}
	if g, _ := get(d, "?direction=down"); !slices.Equal(ids4(g), []int64{a, b, c, d}) || len(g.Edges) != 4 {
		t.Fatalf("down from d = %v %v", ids4(g), g.Edges)
	}
	if g, _ := get(a, "?direction=up"); len(g.Edges) != 4 {
		t.Fatalf("up from a edges = %v", g.Edges)
	}
	_, resp := get(a, "?direction=sideways")
	expectError(t, resp, http.StatusBadRequest, "validation_failed")
	_, resp = get(999, "")
	expectError(t, resp, http.StatusNotFound, "not_found")
}

func TestDeps_ListBlockedFilter(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ids := createTasks(t, fx, "ready", "blocked", "blocker")
	ready, blocked, blocker := ids[0], ids[1], ids[2]
	expectOK(t, addDep(t, fx, blocked, blocker))

	list := func(query string) []int64 {
		resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?sort=created_at&"+query, nil)
		defer func() { _ = resp.Body.Close() }()
		expectOK(t, resp)
		var out []int64
		for _, tk := range decodeTasks(t, resp) {
			out = append(out, tk.ID)
		}
		return out
	}
	if got := list("state=not_done&blocked=false"); !slices.Equal(got, []int64{ready, blocker}) {
		t.Fatalf("ready = %v", got)
	}
	if got := list("blocked=true"); !slices.Equal(got, []int64{blocked}) {
		t.Fatalf("blocked = %v", got)
	}
	expectError(t, doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?blocked=maybe", nil),
		http.StatusBadRequest, "validation_failed")
}

func TestDeps_TaskResponsesCarryEmptyEdgeArrays(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks", map[string]any{"title": "x"})
	defer func() { _ = resp.Body.Close() }()
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for key, want := range map[string]string{"blocked_by": "[]", "blocks": "[]", "blocked": "false"} {
		if string(raw[key]) != want {
			t.Errorf("%s = %s, want %s", key, raw[key], want)
		}
	}
}
