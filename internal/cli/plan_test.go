package cli_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/srliao/tt/internal/task"
)

// planShape is the set of JSON keys in a plan result and its first task, so
// check and apply can be compared without depending on provisional ids.
func planShape(t *testing.T, stdout string) (top, first []string) {
	t.Helper()
	var raw struct {
		IDs   map[string]any   `json:"ids"`
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatal(err)
	}
	var all map[string]any
	_ = json.Unmarshal([]byte(stdout), &all)
	for k := range all {
		top = append(top, k)
	}
	for k := range raw.Tasks[0] {
		first = append(first, k)
	}
	return top, first
}

func TestPlanCheckAndApply_SameResultShape(t *testing.T) {
	f := newFixture(t)
	f.createTag("tt")
	doc := `{"tasks": [
		{"ref": "design", "title": "Design", "due_date": "2026-11-20", "tags": ["tt"], "stage": true},
		{"ref": "impl", "title": "Implement"}],
		"deps": [{"task": "impl", "depends_on": "design"}]}`

	check := f.runStdin(doc, "plan", "check")
	var checked task.PlanResult
	check.ok(t, &checked)
	apply := f.runStdin(doc, "plan", "apply")
	var applied task.PlanResult
	apply.ok(t, &applied)

	ct, cf := planShape(t, check.stdout)
	at, af := planShape(t, apply.stdout)
	if len(ct) != len(at) || len(cf) != len(af) {
		t.Fatalf("shapes differ: check %v %v, apply %v %v", ct, cf, at, af)
	}
	if len(checked.Tasks) != 2 || len(applied.Tasks) != 2 {
		t.Fatalf("task counts %d / %d", len(checked.Tasks), len(applied.Tasks))
	}
	for j, c := range checked.Tasks {
		a := applied.Tasks[j]
		if c.Title != a.Title || c.Blocked != a.Blocked || len(c.BlockedBy) != len(a.BlockedBy) {
			t.Fatalf("task %d differs: check %+v, apply %+v", j, c, a)
		}
	}
	if !applied.Tasks[1].Blocked || applied.Tasks[1].BlockedBy[0] != applied.IDs["design"] {
		t.Fatalf("impl should be blocked by design: %+v", applied.Tasks[1])
	}
}

// A saved snapshot that no longer matches the server passes check, but the
// apply fails as a whole: none of the plan's tasks are written.
func TestPlanApply_StaleSnapshotFailsWithoutPartialWrites(t *testing.T) {
	f := newFixture(t)
	gone := f.createTask("will be deleted")
	snap := f.pullSnapshot()
	if err := f.tasks.Delete(context.Background(), gone.ID); err != nil {
		t.Fatal(err)
	}
	doc := `{"tasks": [{"ref": "a", "title": "A"}, {"ref": "b", "title": "B"}],
		"deps": [{"task": "b", "depends_on": "a"}, {"task": "a", "depends_on": ` + id(gone.ID) + `}]}`

	f.runStdin(doc, "plan", "check", "--snapshot", snap).ok(t, nil)
	f.runStdin(doc, "plan", "apply").fails(t, 2, "not_found")

	all, err := f.tasks.List(context.Background(), task.FilterSort{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("server has %d tasks after failed apply, want 0", len(all))
	}
}

func TestPlanCheck_RuleViolationIsConflict(t *testing.T) {
	f := newFixture(t)
	doc := `{"tasks": [{"ref": "a", "title": "A"}, {"ref": "b", "title": "B"}],
		"deps": [{"task": "a", "depends_on": "b"}, {"task": "b", "depends_on": "a"}]}`

	check := f.runStdin(doc, "plan", "check").fails(t, 3, "conflict")
	apply := f.runStdin(doc, "plan", "apply").fails(t, 3, "conflict")
	if check.Details["rule"] != apply.Details["rule"] {
		t.Fatalf("rule: check %v, apply %v", check.Details["rule"], apply.Details["rule"])
	}
}
