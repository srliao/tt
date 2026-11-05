package task_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/srliao/tt/internal/db"
	"github.com/srliao/tt/internal/tag"
	"github.com/srliao/tt/internal/task"
)

func mustTag(t *testing.T, store *db.Store, ctx context.Context, name string) {
	t.Helper()
	if _, err := tag.New(store).Create(ctx, name); err != nil {
		t.Fatalf("create tag %q: %v", name, err)
	}
}

func decodePlan(t *testing.T, doc string) task.Plan {
	t.Helper()
	var p task.Plan
	if err := json.Unmarshal([]byte(doc), &p); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	return p
}

func planErrorOf(t *testing.T, err error) *task.PlanError {
	t.Helper()
	var pe *task.PlanError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *task.PlanError", err)
	}
	return pe
}

func taskCount(t *testing.T, svc *task.Impl, ctx context.Context) int {
	t.Helper()
	all, err := svc.List(ctx, task.FilterSort{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return len(all)
}

func TestApplyPlan_CreatesTasksAndWiresDeps(t *testing.T) {
	svc, store, ctx := newServiceWithStore(t)
	mustTag(t, store, ctx, "tt")
	existing := mkTasks(t, svc, ctx, "existing")[0]

	plan := decodePlan(t, `{
		"tasks": [
			{"ref": "design", "title": "Design schema", "notes": "n", "due_date": "2026-11-20", "tags": ["TT"], "stage": true},
			{"ref": "impl", "title": "Implement"}
		],
		"deps": [
			{"task": "impl", "depends_on": "design"},
			{"task": "impl", "depends_on": `+jsonInt(existing)+`}
		]
	}`)
	res, err := svc.ApplyPlan(ctx, plan)
	if err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}

	design, impl := res.IDs["design"], res.IDs["impl"]
	if design == 0 || impl == 0 || design == impl || len(res.IDs) != 2 {
		t.Fatalf("ids = %v", res.IDs)
	}
	if len(res.Tasks) < 2 || res.Tasks[0].ID != design || res.Tasks[1].ID != impl {
		t.Fatalf("tasks must lead with plan tasks in doc order, got %v", res.Tasks)
	}

	d := res.Tasks[0]
	if d.Title != "Design schema" || d.Notes != "n" || d.DueDate == nil || *d.DueDate != "2026-11-20" {
		t.Fatalf("design fields = %+v", d)
	}
	if !slices.Equal(d.Tags, []string{"tt"}) || d.StagedOrder == nil {
		t.Fatalf("design tags/stage = %v %v", d.Tags, d.StagedOrder)
	}
	if !slices.Equal(d.Blocks, []int64{impl}) || d.Blocked {
		t.Fatalf("design edges = blocks %v blocked %v", d.Blocks, d.Blocked)
	}

	i := res.Tasks[1]
	if !i.Blocked || !slices.Equal(sorted(i.BlockedBy), sorted([]int64{design, existing})) {
		t.Fatalf("impl edges = blocked_by %v blocked %v", i.BlockedBy, i.Blocked)
	}
	if i.StagedOrder != nil || len(i.Tags) != 0 {
		t.Fatalf("impl must be unstaged and untagged: %+v", i)
	}

	if got := mustGet(t, svc, ctx, existing); !slices.Equal(got.Blocks, []int64{impl}) {
		t.Fatalf("existing blocks = %v", got.Blocks)
	}
}

func jsonInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func sorted(ids []int64) []int64 {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

func TestApplyPlan_CycleAmongNewTasksRollsBack(t *testing.T) {
	svc, ctx := newService(t)
	plan := decodePlan(t, `{
		"tasks": [{"ref": "a", "title": "A"}, {"ref": "b", "title": "B"}],
		"deps": [{"task": "a", "depends_on": "b"}, {"task": "b", "depends_on": "a"}]
	}`)
	_, err := svc.ApplyPlan(ctx, plan)
	rule, details := ruleOf(err)
	if rule != task.RuleCycle {
		t.Fatalf("err = %v, want cycle", err)
	}
	if details["op"] != "add" || details["index"] != 1 {
		t.Fatalf("details = %v, want op=add index=1", details)
	}
	if n := taskCount(t, svc, ctx); n != 0 {
		t.Fatalf("task count = %d after failed plan, want 0", n)
	}
}

func TestApplyPlan_CycleThroughExistingTask(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "x", "y")
	x, y := ids[0], ids[1]
	mustAddDep(t, svc, ctx, x, y)

	plan := decodePlan(t, `{
		"tasks": [{"ref": "n", "title": "N"}],
		"deps": [
			{"task": "n", "depends_on": `+jsonInt(x)+`},
			{"task": `+jsonInt(y)+`, "depends_on": "n"}
		]
	}`)
	_, err := svc.ApplyPlan(ctx, plan)
	if rule, _ := ruleOf(err); rule != task.RuleCycle {
		t.Fatalf("err = %v, want cycle", err)
	}
	if n := taskCount(t, svc, ctx); n != 2 {
		t.Fatalf("task count = %d after failed plan, want 2", n)
	}
	if got := mustGet(t, svc, ctx, y); len(got.BlockedBy) != 0 {
		t.Fatalf("y blocked_by = %v, want none", got.BlockedBy)
	}
}

func TestApplyPlan_RemovalsRunBeforeAdditions(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "x", "y")
	x, y := ids[0], ids[1]
	mustAddDep(t, svc, ctx, x, y)

	plan := decodePlan(t, `{
		"deps": [{"task": `+jsonInt(y)+`, "depends_on": `+jsonInt(x)+`}],
		"deps_remove": [{"task": `+jsonInt(x)+`, "depends_on": `+jsonInt(y)+`}]
	}`)
	res, err := svc.ApplyPlan(ctx, plan)
	if err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	if len(res.Tasks) != 2 || res.Tasks[0].ID != x || !slices.Equal(res.Tasks[1].BlockedBy, []int64{x}) {
		t.Fatalf("tasks = %+v", res.Tasks)
	}
}

func TestApplyPlan_UnknownTaskIDIsNotFound(t *testing.T) {
	svc, ctx := newService(t)
	plan := decodePlan(t, `{
		"tasks": [{"ref": "a", "title": "A"}],
		"deps": [{"task": "a", "depends_on": 999}]
	}`)
	_, err := svc.ApplyPlan(ctx, plan)
	pe := planErrorOf(t, err)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
	if pe.Details["op"] != "add" || pe.Details["index"] != 0 {
		t.Fatalf("details = %v, want op=add index=0", pe.Details)
	}
	if n := taskCount(t, svc, ctx); n != 0 {
		t.Fatalf("task count = %d after failed plan, want 0", n)
	}
}

func TestApplyPlan_RejectsInvalidDocs(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want map[string]any
	}{
		{
			name: "empty ref",
			doc:  `{"tasks": [{"ref": "a", "title": "A"}, {"ref": " ", "title": "B"}]}`,
			want: map[string]any{"field": "ref", "index": 1},
		},
		{
			name: "duplicate ref",
			doc:  `{"tasks": [{"ref": "a", "title": "A"}, {"ref": "a", "title": "B"}]}`,
			want: map[string]any{"field": "ref", "index": 1, "ref": "a"},
		},
		{
			name: "missing title",
			doc:  `{"tasks": [{"ref": "a", "title": ""}]}`,
			want: map[string]any{"field": "title", "index": 0, "ref": "a"},
		},
		{
			name: "bad due date",
			doc:  `{"tasks": [{"ref": "a", "title": "A", "due_date": "tomorrow"}]}`,
			want: map[string]any{"field": "due_date", "index": 0, "ref": "a"},
		},
		{
			name: "unknown ref in deps",
			doc:  `{"tasks": [{"ref": "a", "title": "A"}], "deps": [{"task": "a", "depends_on": "nope"}]}`,
			want: map[string]any{"field": "depends_on", "op": "add", "index": 0, "ref": "nope"},
		},
		{
			name: "unknown ref in deps_remove",
			doc:  `{"deps_remove": [{"task": "ghost", "depends_on": 1}]}`,
			want: map[string]any{"field": "task", "op": "remove", "index": 0, "ref": "ghost"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, ctx := newService(t)
			_, err := svc.ApplyPlan(ctx, decodePlan(t, tc.doc))
			pe := planErrorOf(t, err)
			if errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("err = %v, want validation not not-found", err)
			}
			for k, v := range tc.want {
				if pe.Details[k] != v {
					t.Errorf("details[%q] = %v, want %v (all: %v)", k, pe.Details[k], v, pe.Details)
				}
			}
			if n := taskCount(t, svc, ctx); n != 0 {
				t.Fatalf("task count = %d after failed plan, want 0", n)
			}
		})
	}
}

func TestApplyPlan_CapsTotalOps(t *testing.T) {
	svc, ctx := newService(t)
	var p task.Plan
	for range task.MaxDepBatch + 1 {
		p.Tasks = append(p.Tasks, task.PlanTask{Title: "x"})
	}
	_, err := svc.ApplyPlan(ctx, p)
	if pe := planErrorOf(t, err); pe.Details["max"] != task.MaxDepBatch {
		t.Fatalf("details = %v, want max=%d", pe.Details, task.MaxDepBatch)
	}
}

func TestApplyPlan_UnknownTagListsExisting(t *testing.T) {
	svc, store, ctx := newServiceWithStore(t)
	mustTag(t, store, ctx, "work")
	mustTag(t, store, ctx, "home")

	plan := decodePlan(t, `{"tasks": [
		{"ref": "a", "title": "A", "tags": ["work"]},
		{"ref": "b", "title": "B", "tags": ["home", "errands"]}
	]}`)
	_, err := svc.ApplyPlan(ctx, plan)
	pe := planErrorOf(t, err)
	if pe.Details["field"] != "tags" || pe.Details["index"] != 1 || pe.Details["ref"] != "b" || pe.Details["tag"] != "errands" {
		t.Fatalf("details = %v", pe.Details)
	}
	if got, _ := pe.Details["existing_tags"].([]string); !slices.Equal(got, []string{"home", "work"}) {
		t.Fatalf("existing_tags = %#v, want [home work]", pe.Details["existing_tags"])
	}
	if n := taskCount(t, svc, ctx); n != 0 {
		t.Fatalf("task count = %d after failed plan, want 0", n)
	}
}
