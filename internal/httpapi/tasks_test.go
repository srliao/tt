package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/srliao/tt/internal/task"
)

// doJSON is a tiny test helper that performs an HTTP request with an optional
// JSON body and returns the response. The caller is responsible for closing
// resp.Body.
func doJSON(t *testing.T, method, urlStr string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, urlStr, rdr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, urlStr, err)
	}
	return resp
}

// decodeTask reads a single task.Task off an http.Response body. Tests use
// this so they can assert against typed fields rather than string slices.
func decodeTask(t *testing.T, resp *http.Response) task.Task {
	t.Helper()
	var out task.Task
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	return out
}

func decodeTasks(t *testing.T, resp *http.Response) []task.Task {
	t.Helper()
	var out []task.Task
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	return out
}

func TestTasks_CreateAndGet(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks", map[string]any{
		"title": "buy milk",
		"notes": "from the store",
		"tags":  []string{"errand"},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	created := decodeTask(t, resp)
	if created.Title != "buy milk" {
		t.Fatalf("title = %q", created.Title)
	}
	if len(created.Tags) != 1 || created.Tags[0] != "errand" {
		t.Fatalf("tags = %v", created.Tags)
	}

	resp2 := doJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/tasks/%d", fx.server.URL, created.ID), nil)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", resp2.StatusCode)
	}
	got := decodeTask(t, resp2)
	if got.ID != created.ID {
		t.Fatalf("get id = %d, want %d", got.ID, created.ID)
	}
}

func TestTasks_CreateEmptyTitleIs400(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks", map[string]any{
		"title": "  ",
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

func TestTasks_List(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	for _, title := range []string{"a", "b", "c"} {
		if _, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: title}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	tasks := decodeTasks(t, resp)
	if len(tasks) != 3 {
		t.Fatalf("len = %d, want 3", len(tasks))
	}
}

func TestTasks_ListFilterByState(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	a, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: "a"})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: "b"}); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if _, err := fx.tasks.SetState(context.Background(), a.ID, task.StateDone); err != nil {
		t.Fatalf("done: %v", err)
	}

	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?state=done", nil)
	defer func() { _ = resp.Body.Close() }()
	tasks := decodeTasks(t, resp)
	if len(tasks) != 1 || tasks[0].ID != a.ID {
		t.Fatalf("filtered = %+v", tasks)
	}
}

func TestTasks_ListFilterByTagAND(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	t1, err := fx.tasks.Create(ctx, task.CreateInput{Title: "double"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t2, err := fx.tasks.Create(ctx, task.CreateInput{Title: "single"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	workIDs, err := fx.tags.Resolve(ctx, []string{"work"}, true)
	if err != nil {
		t.Fatalf("resolve work: %v", err)
	}
	urgentIDs, err := fx.tags.Resolve(ctx, []string{"urgent"}, true)
	if err != nil {
		t.Fatalf("resolve urgent: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, t1.ID, append(append([]int64{}, workIDs...), urgentIDs...)); err != nil {
		t.Fatalf("set tags t1: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, t2.ID, workIDs); err != nil {
		t.Fatalf("set tags t2: %v", err)
	}

	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag=work&tag=urgent", nil)
	defer func() { _ = resp.Body.Close() }()
	tasks := decodeTasks(t, resp)
	if len(tasks) != 1 || tasks[0].ID != t1.ID {
		t.Fatalf("AND filter result = %+v", tasks)
	}

	// Explicit tag_mode=all matches the default behavior above.
	respAll := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag=work&tag=urgent&tag_mode=all", nil)
	defer func() { _ = respAll.Body.Close() }()
	tasksAll := decodeTasks(t, respAll)
	if len(tasksAll) != 1 || tasksAll[0].ID != t1.ID {
		t.Fatalf("tag_mode=all filter result = %+v", tasksAll)
	}

	// tag_mode=any returns tasks with at least one of the supplied tags.
	respAny := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag=work&tag=urgent&tag_mode=any", nil)
	defer func() { _ = respAny.Body.Close() }()
	tasksAny := decodeTasks(t, respAny)
	if len(tasksAny) != 2 {
		t.Fatalf("tag_mode=any filter result = %+v (want 2)", tasksAny)
	}
	gotIDs := map[int64]bool{tasksAny[0].ID: true, tasksAny[1].ID: true}
	if !gotIDs[t1.ID] || !gotIDs[t2.ID] {
		t.Fatalf("tag_mode=any missing expected ids: got %+v", tasksAny)
	}
}

func TestTasks_ListInvalidTagMode(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag_mode=bogus", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTasks_ListSearch(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	if _, err := fx.tasks.Create(ctx, task.CreateInput{Title: "buy milk"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := fx.tasks.Create(ctx, task.CreateInput{Title: "buy bread"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?q=milk", nil)
	defer func() { _ = resp.Body.Close() }()
	tasks := decodeTasks(t, resp)
	if len(tasks) != 1 || !strings.Contains(tasks[0].Title, "milk") {
		t.Fatalf("search = %+v", tasks)
	}
}

func TestTasks_ListSortDueDateDesc(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	d1 := "2026-01-01"
	d2 := "2026-12-01"
	if _, err := fx.tasks.Create(ctx, task.CreateInput{Title: "early", DueDate: &d1}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := fx.tasks.Create(ctx, task.CreateInput{Title: "late", DueDate: &d2}); err != nil {
		t.Fatalf("create: %v", err)
	}

	urlStr := fx.server.URL + "/api/v1/tasks?sort=due_date&asc=false"
	resp := doJSON(t, http.MethodGet, urlStr, nil)
	defer func() { _ = resp.Body.Close() }()
	tasks := decodeTasks(t, resp)
	if len(tasks) < 2 {
		t.Fatalf("len = %d", len(tasks))
	}
	if tasks[0].Title != "late" {
		t.Fatalf("first = %q, want \"late\"", tasks[0].Title)
	}
}

func TestTasks_Patch(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	t1, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: "old"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp := doJSON(t, http.MethodPatch, fmt.Sprintf("%s/api/v1/tasks/%d", fx.server.URL, t1.ID), map[string]any{
		"title": "new title",
		"notes": "updated",
		"tags":  []string{"hello"},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	got := decodeTask(t, resp)
	if got.Title != "new title" || got.Notes != "updated" {
		t.Fatalf("got = %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "hello" {
		t.Fatalf("tags = %v", got.Tags)
	}
}

func TestTasks_SetState(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	t1, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: "x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/tasks/%d/state", fx.server.URL, t1.ID), map[string]any{
		"state": "done",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := decodeTask(t, resp)
	if got.State != task.StateDone {
		t.Fatalf("state = %q", got.State)
	}
}

func TestTasks_StageAndUnstage(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	t1, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: "x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stageURL := fmt.Sprintf("%s/api/v1/tasks/%d/stage", fx.server.URL, t1.ID)

	resp := doJSON(t, http.MethodPost, stageURL, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stage status = %d", resp.StatusCode)
	}
	staged := decodeTask(t, resp)
	if staged.StagedOrder == nil {
		t.Fatalf("staged_order is nil after stage")
	}

	resp2 := doJSON(t, http.MethodDelete, stageURL, nil)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("unstage status = %d", resp2.StatusCode)
	}
	unstaged := decodeTask(t, resp2)
	if unstaged.StagedOrder != nil {
		t.Fatalf("staged_order not nil after unstage: %v", *unstaged.StagedOrder)
	}
}

func TestTasks_ReorderMain(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	a, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "b"})
	c, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "c"})

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/reorder", map[string]any{
		"task_id":   c.ID,
		"before_id": a.ID,
		"after_id":  b.ID,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	got := decodeTask(t, resp)
	if !(got.Priority > a.Priority && got.Priority < b.Priority) {
		t.Fatalf("priority = %v, expected between %v and %v", got.Priority, a.Priority, b.Priority)
	}
}

func TestTasks_Delete(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	t1, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: "x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	delURL := fmt.Sprintf("%s/api/v1/tasks/%d", fx.server.URL, t1.ID)
	resp := doJSON(t, http.MethodDelete, delURL, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}

	resp2 := doJSON(t, http.MethodGet, delURL, nil)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("subsequent GET status = %d", resp2.StatusCode)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Error.Code != "not_found" {
		t.Fatalf("code = %q", env.Error.Code)
	}
}

// TestTasks_InvalidStateRejected confirms POST /tasks/:id/state validates
// state before invoking the service.
func TestTasks_InvalidStateRejected(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	t1, err := fx.tasks.Create(context.Background(), task.CreateInput{Title: "x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	resp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/tasks/%d/state", fx.server.URL, t1.ID), map[string]any{
		"state": "weird",
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

// TestTasks_InvalidListFilters confirms URL-param validation surfaces 400s.
func TestTasks_InvalidListFilters(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	cases := []string{
		"state=wat",
		"due=wat",
		"sort=wat",
		"asc=wat",
		"limit=wat",
		"offset=wat",
	}
	for _, qs := range cases {
		u, _ := url.Parse(fx.server.URL + "/api/v1/tasks")
		u.RawQuery = qs
		resp := doJSON(t, http.MethodGet, u.String(), nil)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%q: status = %d", qs, resp.StatusCode)
		}
	}
}
