package cli_test

import (
	"context"
	"slices"
	"testing"

	"github.com/srliao/tt/internal/task"
)

func (f *fixture) setProject(name string, ids ...int64) {
	f.t.Helper()
	if _, err := f.tasks.SetProject(context.Background(), ids, &name); err != nil {
		f.t.Fatal(err)
	}
}

func TestTaskCreate_Project(t *testing.T) {
	f := newFixture(t)

	var got task.Task
	f.run("task", "create", "--title", "x", "--project", " Home ").ok(t, &got)
	if got.Project == nil || *got.Project != "home" {
		t.Fatalf("project = %v, want home", got.Project)
	}
	f.run("task", "create", "--title", "x", "--project", "@x").fails(t, 3, "validation_failed")
}

func TestTaskUpdate_ProjectAndClearProject(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A", "work")

	var got task.Task
	f.run("task", "update", id(a.ID), "--project", "home").ok(t, &got)
	if got.Project == nil || *got.Project != "home" || len(got.Tags) != 1 {
		t.Fatalf("got %+v", got)
	}

	f.run("task", "update", id(a.ID), "--title", "A2").ok(t, &got)
	if got.Project == nil || *got.Project != "home" {
		t.Fatalf("project should be kept: %v", got.Project)
	}

	f.run("task", "update", id(a.ID), "--clear-project").ok(t, &got)
	if got.Project != nil {
		t.Fatalf("project = %q, want null", *got.Project)
	}
	f.run("task", "update", id(a.ID), "--project", "x", "--clear-project").fails(t, 1, "usage")
}

func TestTaskList_ProjectFilter(t *testing.T) {
	f := newFixture(t)
	home := f.createTask("Home")
	work := f.createTask("Work")
	f.createTask("None")
	f.setProject("home", home.ID)
	f.setProject("work", work.ID)

	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"--project", "home"}, []string{"Home"}},
		{[]string{"--project", "home", "--project", "work"}, []string{"Home", "Work"}},
		{[]string{"--project", "@none"}, []string{"None"}},
		{[]string{"--project", "work", "--project", "@none"}, []string{"None", "Work"}},
	}
	for _, tc := range cases {
		var out listOut
		f.run(append([]string{"task", "list"}, tc.args...)...).ok(t, &out)
		got := titles(out.Tasks)
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Fatalf("%v: titles = %v, want %v", tc.args, got, tc.want)
		}
	}

	var out listOut
	f.run("task", "list", "--project", "home").ok(t, &out)
	if p := out.Tasks[0].Project; p == nil || *p != "home" {
		t.Fatalf("compact row project = %v, want home", p)
	}
}

func TestProjectSetAndClear(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A")
	b := f.createTask("B")

	var out listOut
	f.run("project", "set", "Home", id(a.ID), id(b.ID)).ok(t, &out)
	if len(out.Tasks) != 2 {
		t.Fatalf("tasks = %+v", out.Tasks)
	}
	for _, r := range out.Tasks {
		if r.Project == nil || *r.Project != "home" {
			t.Fatalf("row = %+v", r)
		}
	}

	f.run("project", "clear", id(a.ID)).ok(t, &out)
	if len(out.Tasks) != 1 || out.Tasks[0].Project != nil {
		t.Fatalf("tasks = %+v", out.Tasks)
	}

	f.run("project", "set", "work", id(a.ID), "999").fails(t, 2, "not_found")
	if got, _ := f.tasks.Get(context.Background(), a.ID); got.Project != nil {
		t.Fatalf("project changed despite unknown id: %q", *got.Project)
	}
	f.run("project", "set", "@x", id(a.ID)).fails(t, 3, "validation_failed")
	f.run("project", "set", "home").fails(t, 1, "usage")
	f.run("project", "clear").fails(t, 1, "usage")
	f.run("project", "set", "home", "abc").fails(t, 1, "usage")
}

func TestProjectListAndRename(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A")
	b := f.createTask("B")
	f.setProject("home", a.ID)
	f.setProject("house", b.ID)
	if _, err := f.tasks.SetState(context.Background(), b.ID, task.StateDone); err != nil {
		t.Fatal(err)
	}

	var listed struct {
		Projects []task.Project `json:"projects"`
	}
	f.run("project", "list").ok(t, &listed)
	want := []task.Project{{Name: "home", OpenCount: 1, TotalCount: 1}, {Name: "house", OpenCount: 0, TotalCount: 1}}
	if !slices.Equal(listed.Projects, want) {
		t.Fatalf("projects = %+v, want %+v", listed.Projects, want)
	}

	var renamed task.Project
	f.run("project", "rename", "house", "home").ok(t, &renamed)
	if renamed != (task.Project{Name: "home", OpenCount: 1, TotalCount: 2}) {
		t.Fatalf("renamed = %+v", renamed)
	}
	f.run("project", "rename", "nope", "x").fails(t, 2, "not_found")
	f.run("project", "rename", "home", "@x").fails(t, 3, "validation_failed")
	f.run("project", "rename", "home").fails(t, 1, "usage")

	f.run("project", "list").ok(t, &listed)
	if len(listed.Projects) != 1 {
		t.Fatalf("projects after merge = %+v", listed.Projects)
	}
}
