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

// TestTasks_ListLegacyTagParamIgnored confirms that the Phase 6 removal of
// the legacy `tag=` + `tag_mode=` reader leaves those params silently
// ignored — the request returns the full unfiltered list with a 200 status,
// not a 400. Regression guard so a future re-add can't slip in without a
// matching test update.
func TestTasks_ListLegacyTagParamIgnored(t *testing.T) {
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
	t3, err := fx.tasks.Create(ctx, task.CreateInput{Title: "untagged"})
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

	// `?tag=foo&tag=bar` is now silently ignored — every task comes back.
	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag=work&tag=urgent", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body: %s)", resp.StatusCode, string(body))
	}
	tasks := decodeTasks(t, resp)
	if len(tasks) != 3 {
		t.Fatalf("legacy `tag=` should be ignored; got %d tasks (%+v), want 3 (all)", len(tasks), tasks)
	}
	gotIDs := map[int64]bool{}
	for _, tt := range tasks {
		gotIDs[tt.ID] = true
	}
	if !gotIDs[t1.ID] || !gotIDs[t2.ID] || !gotIDs[t3.ID] {
		t.Fatalf("legacy `tag=` should be ignored; missing ids in %+v", tasks)
	}

	// `tag_mode=all` alongside `tag=` is also silently ignored.
	respAll := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag=work&tag=urgent&tag_mode=all", nil)
	defer func() { _ = respAll.Body.Close() }()
	if respAll.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respAll.Body)
		t.Fatalf("tag_mode=all status = %d, want 200 (body: %s)", respAll.StatusCode, string(body))
	}
	tasksAll := decodeTasks(t, respAll)
	if len(tasksAll) != 3 {
		t.Fatalf("legacy `tag=&tag_mode=all` should be ignored; got %d tasks, want 3", len(tasksAll))
	}
}

// TestTasks_ListFilterByTagFilter exercises every branch of the new
// tag_filter URL schema: any / all / @untagged / mixed / impossible / unknown
// tag name / malformed input.
func TestTasks_ListFilterByTagFilter(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	bare, err := fx.tasks.Create(ctx, task.CreateInput{Title: "bare"})
	if err != nil {
		t.Fatalf("create bare: %v", err)
	}
	workTask, err := fx.tasks.Create(ctx, task.CreateInput{Title: "workTask"})
	if err != nil {
		t.Fatalf("create workTask: %v", err)
	}
	dual, err := fx.tasks.Create(ctx, task.CreateInput{Title: "dual"})
	if err != nil {
		t.Fatalf("create dual: %v", err)
	}
	workIDs, err := fx.tags.Resolve(ctx, []string{"work"}, true)
	if err != nil {
		t.Fatalf("resolve work: %v", err)
	}
	urgentIDs, err := fx.tags.Resolve(ctx, []string{"urgent"}, true)
	if err != nil {
		t.Fatalf("resolve urgent: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, workTask.ID, workIDs); err != nil {
		t.Fatalf("set tags workTask: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, dual.ID, append(append([]int64{}, workIDs...), urgentIDs...)); err != nil {
		t.Fatalf("set tags dual: %v", err)
	}

	expectIDs := func(t *testing.T, label, urlSuffix string, want ...int64) {
		t.Helper()
		resp := doJSON(t, http.MethodGet, fx.server.URL+urlSuffix, nil)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s: status = %d body = %s", label, resp.StatusCode, string(body))
		}
		tasks := decodeTasks(t, resp)
		if len(tasks) != len(want) {
			t.Fatalf("%s: got %d tasks (%+v), want %d", label, len(tasks), tasks, len(want))
		}
		got := make(map[int64]bool, len(tasks))
		for _, tt := range tasks {
			got[tt.ID] = true
		}
		for _, id := range want {
			if !got[id] {
				t.Fatalf("%s: missing id %d in %+v", label, id, tasks)
			}
		}
	}

	expectIDs(t, "any:work,urgent", "/api/v1/tasks?tag_filter=any:work,urgent", workTask.ID, dual.ID)
	expectIDs(t, "all:work,urgent", "/api/v1/tasks?tag_filter=all:work,urgent", dual.ID)
	expectIDs(t, "any:@untagged", "/api/v1/tasks?tag_filter=any:@untagged", bare.ID)
	expectIDs(t, "any:@untagged,work", "/api/v1/tasks?tag_filter=any:@untagged,work", bare.ID, workTask.ID, dual.ID)
	// Impossible — must be empty, not 500.
	expectIDs(t, "all:@untagged,work", "/api/v1/tasks?tag_filter=all:@untagged,work")

	// Duplicate names are deduplicated before SQL — `all:work,work` must
	// behave like `all:work` (the All-mode HAVING COUNT(DISTINCT) check
	// would otherwise reject the legit single-tag match).
	expectIDs(t, "all:work,work", "/api/v1/tasks?tag_filter=all:work,work", workTask.ID, dual.ID)

	// Malformed forms degrade to "no filter" (all tasks come back).
	expectIDs(t, "no-colon", "/api/v1/tasks?tag_filter=workurgent", bare.ID, workTask.ID, dual.ID)
	expectIDs(t, "unknown-mode", "/api/v1/tasks?tag_filter=foo:work", bare.ID, workTask.ID, dual.ID)
	expectIDs(t, "empty-list", "/api/v1/tasks?tag_filter=any:", bare.ID, workTask.ID, dual.ID)

	// Unknown but well-formed tag name still surfaces as 400.
	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag_filter=any:nope", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown tag name: status = %d, want 400", resp.StatusCode)
	}
}

func TestTasks_ListFilterByTagsExclude(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	plain, err := fx.tasks.Create(ctx, task.CreateInput{Title: "plain"})
	if err != nil {
		t.Fatalf("create plain: %v", err)
	}
	tagged, err := fx.tasks.Create(ctx, task.CreateInput{Title: "tagged"})
	if err != nil {
		t.Fatalf("create tagged: %v", err)
	}
	skipIDs, err := fx.tags.Resolve(ctx, []string{"skip"}, true)
	if err != nil {
		t.Fatalf("resolve skip: %v", err)
	}
	if err := fx.tasks.SetTagsByID(ctx, tagged.ID, skipIDs); err != nil {
		t.Fatalf("set tags tagged: %v", err)
	}

	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tags_exclude=skip", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	tasks := decodeTasks(t, resp)
	if len(tasks) != 1 || tasks[0].ID != plain.ID {
		t.Fatalf("tags_exclude result = %+v, want [%d]", tasks, plain.ID)
	}
}

// TestTasks_ListUnknownTagModeIsTolerated verifies the Phase 2 behavior
// change: `tag_mode` outside the legacy `tag=` branch is silently ignored
// rather than producing a 400. Stale clients that send `tag_mode=bogus`
// alone (no `tag=`) keep working.
func TestTasks_ListUnknownTagModeIsTolerated(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?tag_mode=bogus", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body: %s)", resp.StatusCode, string(body))
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

	// Newest-first, so the list reads c, b, a. Move a up between c and b.
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/reorder", map[string]any{
		"task_id":   a.ID,
		"before_id": c.ID,
		"after_id":  b.ID,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, string(body))
	}
	got := decodeTask(t, resp)
	if !(got.Priority > c.Priority && got.Priority < b.Priority) {
		t.Fatalf("priority = %v, expected between %v and %v", got.Priority, c.Priority, b.Priority)
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

func TestTasks_BulkTagAddReturnsUpdatedDTOs(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	t1, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "t1"})
	t2, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "t2"})

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/bulk-tag", map[string]any{
		"ids":  []int64{t1.ID, t2.ID},
		"op":   "add",
		"tags": []string{"alpha", "bravo"},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	out := decodeTasks(t, resp)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	gotIDs := map[int64]bool{}
	for _, got := range out {
		gotIDs[got.ID] = true
		want := []string{"alpha", "bravo"}
		if len(got.Tags) != 2 || got.Tags[0] != want[0] || got.Tags[1] != want[1] {
			t.Fatalf("task %d tags = %v, want %v", got.ID, got.Tags, want)
		}
	}
	if !gotIDs[t1.ID] || !gotIDs[t2.ID] {
		t.Fatalf("missing ids in response: %+v", out)
	}
}

func TestTasks_BulkTagSetClearsWithEmptyTags(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	t1, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "t1"})
	tagIDs, _ := fx.tags.Resolve(ctx, []string{"legacy"}, true)
	if err := fx.tasks.SetTagsByID(ctx, t1.ID, tagIDs); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/bulk-tag", map[string]any{
		"ids":  []int64{t1.ID},
		"op":   "set",
		"tags": []string{},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	out := decodeTasks(t, resp)
	if len(out[0].Tags) != 0 {
		t.Fatalf("tags = %v, want []", out[0].Tags)
	}
}

func TestTasks_BulkTagRemoveIgnoresUnknownTagNames(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	t1, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "t1"})
	tagIDs, _ := fx.tags.Resolve(ctx, []string{"alpha"}, true)
	if err := fx.tasks.SetTagsByID(ctx, t1.ID, tagIDs); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Remove a mix of one existing and one unknown tag; the unknown is
	// silently dropped so the task ends up tagless without a 400.
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/bulk-tag", map[string]any{
		"ids":  []int64{t1.ID},
		"op":   "remove",
		"tags": []string{"alpha", "never-existed"},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	out := decodeTasks(t, resp)
	if len(out[0].Tags) != 0 {
		t.Fatalf("tags = %v, want []", out[0].Tags)
	}
}

func TestTasks_BulkTagRemoveAllUnknownTagNamesIsNoOp(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	t1, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "t1"})
	tagIDs, _ := fx.tags.Resolve(ctx, []string{"alpha"}, true)
	if err := fx.tasks.SetTagsByID(ctx, t1.ID, tagIDs); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// All tag names unknown — ResolveExisting drops them all so the service
	// receives TagIDs=[] and must treat it as a silent no-op (200, unchanged).
	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/bulk-tag", map[string]any{
		"ids":  []int64{t1.ID},
		"op":   "remove",
		"tags": []string{"never-existed", "also-never-existed"},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(body))
	}
	out := decodeTasks(t, resp)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if len(out[0].Tags) != 1 || out[0].Tags[0] != "alpha" {
		t.Fatalf("tags = %v, want [alpha] (unchanged)", out[0].Tags)
	}
}

func TestTasks_BulkTagValidation(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty ids", map[string]any{"ids": []int64{}, "op": "add", "tags": []string{"x"}}},
		{"unknown op", map[string]any{"ids": []int64{1}, "op": "bogus", "tags": []string{"x"}}},
		{"add no tags", map[string]any{"ids": []int64{1}, "op": "add", "tags": []string{}}},
		{"remove no tags", map[string]any{"ids": []int64{1}, "op": "remove", "tags": []string{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/bulk-tag", tc.body)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			var env struct {
				Error struct {
					Code, Message string
				}
			}
			if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if env.Error.Code == "" || env.Error.Message == "" {
				t.Fatalf("expected error envelope, got %+v", env)
			}
		})
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
