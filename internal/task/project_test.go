package task_test

import (
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/srliao/tt/internal/task"
)

func ptr(s string) *string { return &s }

func projectOf(t task.Task) string {
	if t.Project == nil {
		return "<nil>"
	}
	return *t.Project
}

func TestCreate_NormalizesProject(t *testing.T) {
	svc, ctx := newService(t)

	got, err := svc.Create(ctx, task.CreateInput{Title: "a", Project: ptr("  Home  ")})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if projectOf(got) != "home" {
		t.Fatalf("project = %s, want home", projectOf(got))
	}

	blank, err := svc.Create(ctx, task.CreateInput{Title: "b", Project: ptr("   ")})
	if err != nil {
		t.Fatalf("Create blank: %v", err)
	}
	if blank.Project != nil {
		t.Fatalf("blank project = %s, want nil", projectOf(blank))
	}
}

func TestCreate_RejectsInvalidProject(t *testing.T) {
	svc, ctx := newService(t)
	for _, p := range []string{"@x", "a,b"} {
		if _, err := svc.Create(ctx, task.CreateInput{Title: "a", Project: ptr(p)}); err == nil {
			t.Fatalf("Create(project=%q) succeeded, want error", p)
		}
	}
}

func TestUpdate_ReplacesProject(t *testing.T) {
	svc, ctx := newService(t)
	a, err := svc.Create(ctx, task.CreateInput{Title: "a", Project: ptr("home")})
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.Update(ctx, a.ID, task.UpdateInput{Title: "a", Project: ptr("Work")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if projectOf(got) != "work" {
		t.Fatalf("project = %s, want work", projectOf(got))
	}

	got, err = svc.Update(ctx, a.ID, task.UpdateInput{Title: "a", Project: ptr("")})
	if err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	if got.Project != nil {
		t.Fatalf("project = %s, want nil", projectOf(got))
	}

	if _, err := svc.Update(ctx, a.ID, task.UpdateInput{Title: "a", Project: ptr("@x")}); err == nil {
		t.Fatal("Update(project=@x) succeeded, want error")
	}
}

func TestSetProject_SetsAndClearsInRequestOrder(t *testing.T) {
	svc, ctx := newService(t)
	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})

	got, err := svc.SetProject(ctx, []int64{b.ID, a.ID}, ptr(" Home "))
	if err != nil {
		t.Fatalf("SetProject: %v", err)
	}
	if len(got) != 2 || got[0].ID != b.ID || got[1].ID != a.ID {
		t.Fatalf("got ids %v, want [%d %d]", got, b.ID, a.ID)
	}
	for _, g := range got {
		if projectOf(g) != "home" {
			t.Fatalf("task %d project = %s, want home", g.ID, projectOf(g))
		}
	}

	got, err = svc.SetProject(ctx, []int64{a.ID}, nil)
	if err != nil {
		t.Fatalf("SetProject clear: %v", err)
	}
	if got[0].Project != nil {
		t.Fatalf("project = %s, want nil", projectOf(got[0]))
	}
}

func TestSetProject_UnknownIDChangesNothing(t *testing.T) {
	svc, ctx := newService(t)
	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})

	_, err := svc.SetProject(ctx, []int64{a.ID, 999}, ptr("home"))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
	reloaded, _ := svc.Get(ctx, a.ID)
	if reloaded.Project != nil {
		t.Fatalf("project = %s, want nil after failed bulk set", projectOf(reloaded))
	}
}

func TestSetProject_Validation(t *testing.T) {
	svc, ctx := newService(t)
	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	if _, err := svc.SetProject(ctx, nil, ptr("home")); err == nil {
		t.Fatal("empty ids succeeded, want error")
	}
	if _, err := svc.SetProject(ctx, []int64{a.ID}, ptr("a,b")); err == nil {
		t.Fatal("project a,b succeeded, want error")
	}
}

func TestListProjects_CountsOpenAndTotal(t *testing.T) {
	svc, ctx := newService(t)
	mk := func(title, project string, st task.State) {
		t.Helper()
		created, err := svc.Create(ctx, task.CreateInput{Title: title, Project: ptr(project)})
		if err != nil {
			t.Fatal(err)
		}
		if st != task.StateNotDone {
			if _, err := svc.SetState(ctx, created.ID, st); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("a", "work", task.StateNotDone)
	mk("b", "work", task.StateDone)
	mk("c", "home", task.StateDone)
	mk("d", "home", task.StateCancelled)
	mk("e", "", task.StateNotDone)

	got, err := svc.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	want := []task.Project{
		{Name: "home", OpenCount: 0, TotalCount: 2},
		{Name: "work", OpenCount: 1, TotalCount: 2},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRenameProject_MovesAndMerges(t *testing.T) {
	svc, ctx := newService(t)
	a, _ := svc.Create(ctx, task.CreateInput{Title: "a", Project: ptr("home")})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b", Project: ptr("house")})
	c, _ := svc.Create(ctx, task.CreateInput{Title: "c", Project: ptr("work")})

	if err := svc.RenameProject(ctx, " HOUSE ", "Home"); err != nil {
		t.Fatalf("RenameProject merge: %v", err)
	}
	for _, id := range []int64{a.ID, b.ID} {
		got, _ := svc.Get(ctx, id)
		if projectOf(got) != "home" {
			t.Fatalf("task %d project = %s, want home", id, projectOf(got))
		}
	}
	if got, _ := svc.Get(ctx, c.ID); projectOf(got) != "work" {
		t.Fatalf("unrelated task moved to %s", projectOf(got))
	}

	if err := svc.RenameProject(ctx, "home", "chores"); err != nil {
		t.Fatalf("RenameProject: %v", err)
	}
	projects, _ := svc.ListProjects(ctx)
	if len(projects) != 2 || projects[0].Name != "chores" || projects[0].TotalCount != 2 {
		t.Fatalf("projects = %+v", projects)
	}
}

func TestRenameProject_Errors(t *testing.T) {
	svc, ctx := newService(t)
	if _, err := svc.Create(ctx, task.CreateInput{Title: "a", Project: ptr("home")}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RenameProject(ctx, "nope", "x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unused from: err = %v, want sql.ErrNoRows", err)
	}
	for _, to := range []string{"", "  ", "@x", "a,b"} {
		if err := svc.RenameProject(ctx, "home", to); err == nil {
			t.Fatalf("rename to %q succeeded, want error", to)
		}
	}
}

func TestList_FilterByProject(t *testing.T) {
	svc, ctx := newService(t)
	home, _ := svc.Create(ctx, task.CreateInput{Title: "home", Project: ptr("home")})
	work, _ := svc.Create(ctx, task.CreateInput{Title: "work", Project: ptr("work")})
	none, _ := svc.Create(ctx, task.CreateInput{Title: "none"})
	if _, err := svc.SetState(ctx, work.ID, task.StateDone); err != nil {
		t.Fatal(err)
	}

	ids := func(f task.FilterSort) []int64 {
		t.Helper()
		got, err := svc.List(ctx, f)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		out := make([]int64, 0, len(got))
		for _, g := range got {
			out = append(out, g.ID)
		}
		slices.Sort(out)
		return out
	}
	sorted := func(in ...int64) []int64 { slices.Sort(in); return in }

	cases := []struct {
		name string
		f    task.FilterSort
		want []int64
	}{
		{"one name", task.FilterSort{Projects: task.ProjectFilter{Names: []string{"home"}}}, sorted(home.ID)},
		{"two names", task.FilterSort{Projects: task.ProjectFilter{Names: []string{"home", "work"}}}, sorted(home.ID, work.ID)},
		{"none only", task.FilterSort{Projects: task.ProjectFilter{IncludeNone: true}}, sorted(none.ID)},
		{"name or none", task.FilterSort{Projects: task.ProjectFilter{Names: []string{"home"}, IncludeNone: true}}, sorted(home.ID, none.ID)},
		{"ANDs with state", task.FilterSort{
			States:   []task.State{task.StateNotDone},
			Projects: task.ProjectFilter{Names: []string{"home", "work"}},
		}, sorted(home.ID)},
	}
	for _, tc := range cases {
		if got := ids(tc.f); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
