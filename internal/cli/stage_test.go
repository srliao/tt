package cli_test

import (
	"context"
	"slices"
	"testing"

	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

// stageFixture leaves the stage reading A, B, C and returns those tasks in
// that order. Staging puts a task on top, so they are staged C, B, A.
func stageFixture(t *testing.T) (*fixture, []task.Task) {
	t.Helper()
	f := newFixture(t)
	staged := make([]task.Task, 3)
	for i, title := range []string{"C", "B", "A"} {
		tk := f.createTask(title)
		if _, err := f.tasks.Stage(context.Background(), tk.ID); err != nil {
			t.Fatal(err)
		}
		staged[2-i] = tk
	}
	f.createTask("Unstaged")
	return f, staged
}

func stageTitles(t *testing.T, f *fixture) []string {
	t.Helper()
	var out listOut
	f.run("stage", "list").ok(t, &out)
	return titles(out.Tasks)
}

func TestStageList_InStageOrder(t *testing.T) {
	f, _ := stageFixture(t)
	if got := stageTitles(t, f); !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Fatalf("stage = %v", got)
	}
}

func TestStageReorder(t *testing.T) {
	f, s := stageFixture(t)
	a, b, c := s[0], s[1], s[2]

	var got task.Task
	f.run("stage", "reorder", id(c.ID), "--before", id(a.ID)).ok(t, &got)
	if got.ID != c.ID {
		t.Fatalf("got %+v", got)
	}
	if order := stageTitles(t, f); !slices.Equal(order, []string{"C", "A", "B"}) {
		t.Fatalf("after --before: %v", order)
	}

	f.run("stage", "reorder", id(c.ID), "--after", id(a.ID)).ok(t, nil)
	if order := stageTitles(t, f); !slices.Equal(order, []string{"A", "C", "B"}) {
		t.Fatalf("after --after: %v", order)
	}

	f.run("stage", "reorder", id(a.ID), "--bottom").ok(t, nil)
	if order := stageTitles(t, f); !slices.Equal(order, []string{"C", "B", "A"}) {
		t.Fatalf("after --bottom: %v", order)
	}

	f.run("stage", "reorder", id(a.ID), "--top").ok(t, nil)
	if order := stageTitles(t, f); !slices.Equal(order, []string{"A", "C", "B"}) {
		t.Fatalf("after --top: %v", order)
	}

	f.run("stage", "reorder", id(a.ID)).fails(t, 1, "usage")
	f.run("stage", "reorder", id(a.ID), "--top", "--bottom").fails(t, 1, "usage")
	f.run("stage", "reorder", id(b.ID), "--before", "999").fails(t, 2, "not_found")
	f.run("stage", "reorder", "999", "--top").fails(t, 2, "not_found")
	loose := f.createTask("Loose")
	f.run("stage", "reorder", id(loose.ID), "--top").fails(t, 3, "validation_failed")
	f.run("stage", "reorder", id(a.ID), "--after", id(loose.ID)).fails(t, 3, "validation_failed")
}

func TestStageClearFinished(t *testing.T) {
	f, s := stageFixture(t)
	if _, err := f.tasks.SetState(context.Background(), s[1].ID, task.StateDone); err != nil {
		t.Fatal(err)
	}

	var out struct {
		OK bool `json:"ok"`
	}
	f.run("stage", "clear-finished").ok(t, &out)
	if !out.OK {
		t.Fatalf("out = %+v", out)
	}
	if got := stageTitles(t, f); !slices.Equal(got, []string{"A", "C"}) {
		t.Fatalf("stage = %v", got)
	}
}

func TestTagListCreate(t *testing.T) {
	f := newFixture(t)
	f.createTask("A", "work")

	var created tag.Tag
	f.run("tag", "create", "Home").ok(t, &created)
	if created.ID == 0 || created.Name != "home" {
		t.Fatalf("created = %+v", created)
	}

	var out struct {
		Tags []tag.TagWithCount `json:"tags"`
	}
	f.run("tag", "list").ok(t, &out)
	counts := map[string]int64{}
	for _, tg := range out.Tags {
		counts[tg.Name] = tg.Count
	}
	if len(counts) != 2 || counts["work"] != 1 || counts["home"] != 0 {
		t.Fatalf("tags = %+v", out.Tags)
	}

	f.run("tag", "create", "@bad").fails(t, 3, "validation_failed")
}
