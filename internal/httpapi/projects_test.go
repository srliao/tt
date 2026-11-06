package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/srliao/tt/internal/task"
)

func strPtr(s string) *string { return &s }

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body = %s", resp.StatusCode, want, body)
	}
}

func TestTasks_CreateAndPatchProject(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks", map[string]any{"title": "a", "project": "  Home  "})
	defer func() { _ = resp.Body.Close() }()
	wantStatus(t, resp, http.StatusCreated)
	created := decodeTask(t, resp)
	if created.Project == nil || *created.Project != "home" {
		t.Fatalf("project = %v, want home", created.Project)
	}

	patch := doJSON(t, http.MethodPatch, fx.server.URL+fmt.Sprintf("/api/v1/tasks/%d", created.ID), map[string]any{"title": "a", "project": ""})
	defer func() { _ = patch.Body.Close() }()
	wantStatus(t, patch, http.StatusOK)
	if got := decodeTask(t, patch); got.Project != nil {
		t.Fatalf("project = %q, want null", *got.Project)
	}

	for _, p := range []string{"@x", "a,b"} {
		bad := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks", map[string]any{"title": "a", "project": p})
		_ = bad.Body.Close()
		if bad.StatusCode != http.StatusBadRequest {
			t.Fatalf("create project %q: status = %d, want 400", p, bad.StatusCode)
		}
		bad = doJSON(t, http.MethodPatch, fx.server.URL+fmt.Sprintf("/api/v1/tasks/%d", created.ID), map[string]any{"title": "a", "project": p})
		_ = bad.Body.Close()
		if bad.StatusCode != http.StatusBadRequest {
			t.Fatalf("patch project %q: status = %d, want 400", p, bad.StatusCode)
		}
	}
}

func TestTasks_SetProject(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ctx := context.Background()
	a, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "b"})

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/project", map[string]any{"ids": []int64{b.ID, a.ID}, "project": "Home"})
	defer func() { _ = resp.Body.Close() }()
	wantStatus(t, resp, http.StatusOK)
	got := decodeTasks(t, resp)
	if len(got) != 2 || got[0].ID != b.ID || got[1].ID != a.ID {
		t.Fatalf("got %+v, want tasks [%d %d]", got, b.ID, a.ID)
	}
	for _, g := range got {
		if g.Project == nil || *g.Project != "home" {
			t.Fatalf("task %d project = %v", g.ID, g.Project)
		}
	}

	clear := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/project", map[string]any{"ids": []int64{a.ID}, "project": nil})
	defer func() { _ = clear.Body.Close() }()
	wantStatus(t, clear, http.StatusOK)
	if got := decodeTasks(t, clear); got[0].Project != nil {
		t.Fatalf("project = %q, want null", *got[0].Project)
	}

	unknown := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/project", map[string]any{"ids": []int64{a.ID, 999}, "project": "work"})
	defer func() { _ = unknown.Body.Close() }()
	wantStatus(t, unknown, http.StatusNotFound)
	if reloaded, _ := fx.tasks.Get(ctx, a.ID); reloaded.Project != nil {
		t.Fatalf("task %d changed to %q despite unknown id", a.ID, *reloaded.Project)
	}

	for name, body := range map[string]map[string]any{
		"empty ids":  {"ids": []int64{}, "project": "x"},
		"bad name":   {"ids": []int64{a.ID}, "project": "@x"},
		"bad json":   nil,
		"comma name": {"ids": []int64{a.ID}, "project": "a,b"},
	} {
		var resp *http.Response
		if body == nil {
			req, _ := http.NewRequest(http.MethodPost, fx.server.URL+"/api/v1/tasks/project", strings.NewReader("{"))
			resp, _ = http.DefaultClient.Do(req)
		} else {
			resp = doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/tasks/project", body)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", name, resp.StatusCode)
		}
	}
}

func TestProjects_ListAndRename(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ctx := context.Background()
	open, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "a", Project: strPtr("work")})
	done, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "b", Project: strPtr("home")})
	_, _ = fx.tasks.Create(ctx, task.CreateInput{Title: "c"})
	if _, err := fx.tasks.SetState(ctx, done.ID, task.StateDone); err != nil {
		t.Fatal(err)
	}

	list := func() []task.Project {
		t.Helper()
		resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/projects", nil)
		defer func() { _ = resp.Body.Close() }()
		wantStatus(t, resp, http.StatusOK)
		var out []task.Project
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	want := []task.Project{{Name: "home", OpenCount: 0, TotalCount: 1}, {Name: "work", OpenCount: 1, TotalCount: 1}}
	if got := list(); !slices.Equal(got, want) {
		t.Fatalf("projects = %+v, want %+v", got, want)
	}

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/projects/rename", map[string]any{"from": "Work", "to": "home"})
	defer func() { _ = resp.Body.Close() }()
	wantStatus(t, resp, http.StatusOK)
	var merged task.Project
	if err := json.NewDecoder(resp.Body).Decode(&merged); err != nil {
		t.Fatal(err)
	}
	if merged != (task.Project{Name: "home", OpenCount: 1, TotalCount: 2}) {
		t.Fatalf("rename response = %+v", merged)
	}
	if got, _ := fx.tasks.Get(ctx, open.ID); got.Project == nil || *got.Project != "home" {
		t.Fatalf("task %d project = %v, want home", open.ID, got.Project)
	}

	cases := []struct {
		body map[string]any
		want int
	}{
		{map[string]any{"from": "nope", "to": "x"}, http.StatusNotFound},
		{map[string]any{"from": "home", "to": ""}, http.StatusBadRequest},
		{map[string]any{"from": "home", "to": "@x"}, http.StatusBadRequest},
		{map[string]any{"from": "home", "to": "a,b"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/projects/rename", tc.body)
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("rename %v: status = %d, want %d", tc.body, resp.StatusCode, tc.want)
		}
	}
}

func TestTasks_ListFilterByProject(t *testing.T) {
	t.Parallel()
	fx := newTestServer(t, nil)
	ctx := context.Background()
	home, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "home", Project: strPtr("home")})
	work, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "work", Project: strPtr("work")})
	none, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "none"})
	tagged, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "tagged", Project: strPtr("home")})
	urgent, err := fx.tags.Resolve(ctx, []string{"urgent"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.tasks.SetTagsByID(ctx, tagged.ID, urgent); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.tasks.SetState(ctx, work.ID, task.StateDone); err != nil {
		t.Fatal(err)
	}

	expect := func(query string, want ...int64) {
		t.Helper()
		resp := doJSON(t, http.MethodGet, fx.server.URL+"/api/v1/tasks?"+query, nil)
		defer func() { _ = resp.Body.Close() }()
		wantStatus(t, resp, http.StatusOK)
		var got []int64
		for _, g := range decodeTasks(t, resp) {
			got = append(got, g.ID)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("%s: got %v, want %v", query, got, want)
		}
	}
	expect("project=home", home.ID, tagged.ID)
	expect("project=HOME,work", home.ID, work.ID, tagged.ID)
	expect("project=@none", none.ID)
	expect("project=home,@none", home.ID, none.ID, tagged.ID)
	expect("project=home,work&state=not_done", home.ID, tagged.ID)
	expect("project=home&tag_filter=any:urgent", tagged.ID)
	expect("project=home&limit=1&offset=1", home.ID)
	expect("project=nope")
}
