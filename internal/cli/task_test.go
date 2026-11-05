package cli_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/srliao/tt/internal/task"
)

func id(n int64) string { return strconv.FormatInt(n, 10) }

func TestTaskCreate_UnknownTagListsExisting(t *testing.T) {
	f := newFixture(t)
	f.createTag("work")

	e := f.run("task", "create", "--title", "x", "--tag", "wrok").fails(t, 3, "validation_failed")

	existing, _ := e.Details["existing"].([]any)
	if len(existing) != 1 || existing[0] != "work" {
		t.Fatalf("details.existing = %v", e.Details["existing"])
	}
	if got, _ := f.tasks.List(context.Background(), task.FilterSort{}); len(got) != 0 {
		t.Fatalf("task was created despite unknown tag: %v", got)
	}
}

func TestTaskCreate_RejectsNonISODate(t *testing.T) {
	f := newFixture(t)
	f.run("task", "create", "--title", "x", "--due", "tomorrow").fails(t, 3, "validation_failed")
}

func TestUsageErrors(t *testing.T) {
	f := newFixture(t)
	f.run("task", "create").fails(t, 1, "usage")
	f.run("task", "get", "abc").fails(t, 1, "usage")
	f.run("task", "get").fails(t, 1, "usage")
	f.run("task", "bogus").fails(t, 1, "usage")
	f.run("task").fails(t, 1, "usage")
	f.run("task", "create", "--title", "x", "--nope").fails(t, 1, "usage")
}

func TestTaskGet(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A")

	var got task.Task
	f.run("task", "get", id(a.ID)).ok(t, &got)
	if got.ID != a.ID || got.Title != "A" {
		t.Fatalf("got %+v", got)
	}

	f.run("task", "get", "999").fails(t, 2, "not_found")
}

type compactRow struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	State   string   `json:"state"`
	DueDate *string  `json:"due_date"`
	Tags    []string `json:"tags"`
	Blocked bool     `json:"blocked"`
	Staged  bool     `json:"staged"`
	Notes   *string  `json:"notes"`
}

type listOut struct {
	Tasks      []compactRow `json:"tasks"`
	NextCursor string       `json:"next_cursor"`
}

func titles(rows []compactRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}

func TestTaskList_OpenCompactByDefault(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A", "work")
	done := f.createTask("Done")
	ctx := context.Background()
	if _, err := f.tasks.SetState(ctx, done.ID, task.StateDone); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tasks.Stage(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	var out listOut
	r := f.run("task", "list")
	r.ok(t, &out)
	if got := titles(out.Tasks); len(got) != 1 || got[0] != "A" {
		t.Fatalf("titles = %v", got)
	}
	row := out.Tasks[0]
	if row.State != "not_done" || !row.Staged || row.Blocked || len(row.Tags) != 1 || row.Tags[0] != "work" {
		t.Fatalf("row = %+v", row)
	}
	if row.Notes != nil {
		t.Fatalf("compact row should not carry notes: %s", r.stdout)
	}
	if out.NextCursor != "" {
		t.Fatalf("next_cursor = %q, want none", out.NextCursor)
	}

	var full struct {
		Tasks []task.Task `json:"tasks"`
	}
	f.run("task", "list", "--full").ok(t, &full)
	if len(full.Tasks) != 1 || full.Tasks[0].CreatedAt.IsZero() {
		t.Fatalf("full = %+v", full)
	}
}

func TestTaskList_ReadyAndBlocked(t *testing.T) {
	f := newFixture(t)
	blocker := f.createTask("Blocker")
	waiting := f.createTask("Waiting")
	if _, err := f.tasks.AddDep(context.Background(), waiting.ID, blocker.ID); err != nil {
		t.Fatal(err)
	}

	var ready, blocked listOut
	f.run("task", "list", "--ready").ok(t, &ready)
	f.run("task", "list", "--blocked").ok(t, &blocked)
	if got := titles(ready.Tasks); len(got) != 1 || got[0] != "Blocker" {
		t.Fatalf("ready = %v", got)
	}
	if got := titles(blocked.Tasks); len(got) != 1 || got[0] != "Waiting" || !blocked.Tasks[0].Blocked {
		t.Fatalf("blocked = %v", blocked.Tasks)
	}
	f.run("task", "list", "--ready", "--blocked").fails(t, 1, "usage")
}

func TestTaskList_PagesWithCursor(t *testing.T) {
	f := newFixture(t)
	for _, title := range []string{"A", "B", "C"} {
		f.createTask(title)
	}

	var p1, p2 listOut
	f.run("task", "list", "--limit", "2").ok(t, &p1)
	if got := titles(p1.Tasks); len(got) != 2 || p1.NextCursor == "" {
		t.Fatalf("page 1 = %v cursor %q", got, p1.NextCursor)
	}
	f.run("task", "list", "--limit", "2", "--cursor", p1.NextCursor).ok(t, &p2)
	if got := titles(p2.Tasks); len(got) != 1 || p2.NextCursor != "" {
		t.Fatalf("page 2 = %v cursor %q", got, p2.NextCursor)
	}
	seen := map[string]bool{}
	for _, r := range append(p1.Tasks, p2.Tasks...) {
		seen[r.Title] = true
	}
	if len(seen) != 3 {
		t.Fatalf("pages overlap or miss tasks: %v", seen)
	}
}

func TestTaskList_DefaultLimitIs50(t *testing.T) {
	f := newFixture(t)
	for i := range 51 {
		f.createTask("t" + strconv.Itoa(i))
	}
	var out listOut
	f.run("task", "list").ok(t, &out)
	if len(out.Tasks) != 50 || out.NextCursor == "" {
		t.Fatalf("len = %d cursor %q", len(out.Tasks), out.NextCursor)
	}
}

func TestTaskList_TagFilter(t *testing.T) {
	f := newFixture(t)
	f.createTask("A", "work")
	f.createTask("B", "home")

	var out listOut
	f.run("task", "list", "--tag", "work").ok(t, &out)
	if got := titles(out.Tasks); len(got) != 1 || got[0] != "A" {
		t.Fatalf("titles = %v", got)
	}
	f.run("task", "list", "--tag", "nope").fails(t, 3, "validation_failed")
}

func TestTaskUpdate_ChangesOnlyGivenFields(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A", "work")
	if _, err := f.tasks.Update(context.Background(), a.ID, task.UpdateInput{Title: "A", Notes: "keep me"}); err != nil {
		t.Fatal(err)
	}

	var got task.Task
	f.run("task", "update", id(a.ID), "--title", "A2", "--due", "2026-12-01").ok(t, &got)
	if got.Title != "A2" || got.Notes != "keep me" || got.DueDate == nil || *got.DueDate != "2026-12-01" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "work" {
		t.Fatalf("tags should be untouched: %v", got.Tags)
	}

	f.run("task", "update", id(a.ID), "--clear-due").ok(t, &got)
	if got.DueDate != nil {
		t.Fatalf("due should be cleared: %v", *got.DueDate)
	}
	f.run("task", "update", id(a.ID)).fails(t, 1, "usage")
	f.run("task", "update", "999", "--title", "x").fails(t, 2, "not_found")
}

func TestTaskState(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A")

	var got task.Task
	f.run("task", "state", id(a.ID), "done").ok(t, &got)
	if got.State != task.StateDone || got.CompletedAt == nil {
		t.Fatalf("got %+v", got)
	}
	f.run("task", "state", id(a.ID), "finished").fails(t, 3, "validation_failed")
	f.run("task", "state", "999", "done").fails(t, 2, "not_found")
}

func TestTaskStageUnstage(t *testing.T) {
	f := newFixture(t)
	a := f.createTask("A")

	var got task.Task
	f.run("task", "stage", id(a.ID)).ok(t, &got)
	if got.StagedOrder == nil {
		t.Fatalf("not staged: %+v", got)
	}
	f.run("task", "unstage", id(a.ID)).ok(t, &got)
	if got.StagedOrder != nil {
		t.Fatalf("still staged: %+v", got)
	}
	f.run("task", "stage", "999").fails(t, 2, "not_found")
}

func TestTaskTagUntag(t *testing.T) {
	f := newFixture(t)
	f.createTag("work")
	f.createTag("home")
	a := f.createTask("A")

	var got task.Task
	f.run("task", "tag", id(a.ID), "work", "home").ok(t, &got)
	if len(got.Tags) != 2 {
		t.Fatalf("tags = %v", got.Tags)
	}
	f.run("task", "untag", id(a.ID), "home").ok(t, &got)
	if len(got.Tags) != 1 || got.Tags[0] != "work" {
		t.Fatalf("tags = %v", got.Tags)
	}

	e := f.run("task", "tag", id(a.ID), "nope").fails(t, 3, "validation_failed")
	if _, ok := e.Details["existing"]; !ok {
		t.Fatalf("details = %v", e.Details)
	}
	f.run("task", "tag", "999", "work").fails(t, 2, "not_found")
}
