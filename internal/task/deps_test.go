package task_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/srliao/tt/internal/task"
)

// mkTasks creates one task per title and returns their ids in order.
func mkTasks(t *testing.T, svc *task.Impl, ctx context.Context, titles ...string) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(titles))
	for _, title := range titles {
		created, err := svc.Create(ctx, task.CreateInput{Title: title})
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		ids = append(ids, created.ID)
	}
	return ids
}

func mustAddDep(t *testing.T, svc *task.Impl, ctx context.Context, taskID, dependsOnID int64) {
	t.Helper()
	if _, err := svc.AddDep(ctx, taskID, dependsOnID); err != nil {
		t.Fatalf("AddDep(%d, %d): %v", taskID, dependsOnID, err)
	}
}

func mustSetState(t *testing.T, svc *task.Impl, ctx context.Context, id int64, st task.State) {
	t.Helper()
	if _, err := svc.SetState(ctx, id, st); err != nil {
		t.Fatalf("SetState(%d, %s): %v", id, st, err)
	}
}

func mustGet(t *testing.T, svc *task.Impl, ctx context.Context, id int64) task.Task {
	t.Helper()
	got, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get(%d): %v", id, err)
	}
	return got
}

// ruleOf returns the violated rule name, or "" when err is not a RuleError.
func ruleOf(err error) (string, map[string]any) {
	var re *task.RuleError
	if !errors.As(err, &re) {
		return "", nil
	}
	return re.Rule, re.Details
}

func TestAddDep_ReportsEdgeOnBothEnds(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c")
	a, b, c := ids[0], ids[1], ids[2]

	got, err := svc.AddDep(ctx, a, c)
	if err != nil {
		t.Fatalf("AddDep: %v", err)
	}
	mustAddDep(t, svc, ctx, a, b)

	if !slices.Equal(got.BlockedBy, []int64{c}) || !got.Blocked {
		t.Fatalf("AddDep returned blocked_by=%v blocked=%v", got.BlockedBy, got.Blocked)
	}
	ta := mustGet(t, svc, ctx, a)
	if !slices.Equal(ta.BlockedBy, []int64{b, c}) || len(ta.Blocks) != 0 || !ta.Blocked {
		t.Fatalf("a: blocked_by=%v blocks=%v blocked=%v", ta.BlockedBy, ta.Blocks, ta.Blocked)
	}
	tc := mustGet(t, svc, ctx, c)
	if !slices.Equal(tc.Blocks, []int64{a}) || len(tc.BlockedBy) != 0 || tc.Blocked {
		t.Fatalf("c: blocked_by=%v blocks=%v blocked=%v", tc.BlockedBy, tc.Blocks, tc.Blocked)
	}
}

func TestAddDep_R1_BlockedTaskMustBeOpen(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "done", "cancelled", "open", "blocker-done", "blocker-cancelled")
	done, cancelled, open, bDone, bCancelled := ids[0], ids[1], ids[2], ids[3], ids[4]
	mustSetState(t, svc, ctx, done, task.StateDone)
	mustSetState(t, svc, ctx, cancelled, task.StateCancelled)
	mustSetState(t, svc, ctx, bDone, task.StateDone)
	mustSetState(t, svc, ctx, bCancelled, task.StateCancelled)

	for _, id := range []int64{done, cancelled} {
		_, err := svc.AddDep(ctx, id, open)
		if rule, _ := ruleOf(err); rule != task.RuleBlockedTaskNotOpen {
			t.Fatalf("AddDep(%d, open) err = %v, want %s", id, err, task.RuleBlockedTaskNotOpen)
		}
	}
	mustAddDep(t, svc, ctx, open, bDone)
	mustAddDep(t, svc, ctx, open, bCancelled)
}

func TestAddDep_R2_RejectsSelfLinkAndCycle(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c")
	a, b, c := ids[0], ids[1], ids[2]

	_, err := svc.AddDep(ctx, a, a)
	if rule, d := ruleOf(err); rule != task.RuleCycle || !slices.Equal(d["path"].([]int64), []int64{a, a}) {
		t.Fatalf("self link err = %v, details = %v", err, d)
	}

	mustAddDep(t, svc, ctx, a, b)
	mustAddDep(t, svc, ctx, b, c)
	_, err = svc.AddDep(ctx, c, a)
	rule, d := ruleOf(err)
	if rule != task.RuleCycle {
		t.Fatalf("cycle err = %v", err)
	}
	if got := d["path"].([]int64); !slices.Equal(got, []int64{c, a, b, c}) {
		t.Fatalf("path = %v, want %v", got, []int64{c, a, b, c})
	}
}

func TestBlocked_R3_CancelledBlockerStillBlocks(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c")
	a, b, c := ids[0], ids[1], ids[2]
	mustAddDep(t, svc, ctx, a, b)
	mustSetState(t, svc, ctx, b, task.StateCancelled)
	if !mustGet(t, svc, ctx, a).Blocked {
		t.Fatalf("a with only a cancelled blocker should be blocked")
	}

	mustSetState(t, svc, ctx, b, task.StateDone)
	mustAddDep(t, svc, ctx, a, c)
	mustSetState(t, svc, ctx, c, task.StateDone)
	if mustGet(t, svc, ctx, a).Blocked {
		t.Fatalf("a with every blocker done should not be blocked")
	}
}

func TestSetState_R4_BlockedTaskCannotBeDone(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c", "d")
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]
	mustAddDep(t, svc, ctx, a, b)
	mustAddDep(t, svc, ctx, a, c)
	mustAddDep(t, svc, ctx, a, d)
	mustSetState(t, svc, ctx, c, task.StateCancelled)
	mustSetState(t, svc, ctx, d, task.StateDone)

	_, err := svc.SetState(ctx, a, task.StateDone)
	rule, details := ruleOf(err)
	if rule != task.RuleBlocked || !slices.Equal(details["blockers"].([]int64), []int64{b, c}) {
		t.Fatalf("err = %v, details = %v", err, details)
	}
	if st := mustGet(t, svc, ctx, a).State; st != task.StateNotDone {
		t.Fatalf("state = %s after rejected transition", st)
	}

	mustSetState(t, svc, ctx, a, task.StateCancelled)
}

func TestSetState_R5_ReopeningBlockerLeavesDependentsDone(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b")
	a, b := ids[0], ids[1]
	mustAddDep(t, svc, ctx, a, b)
	mustSetState(t, svc, ctx, b, task.StateDone)
	mustSetState(t, svc, ctx, a, task.StateDone)

	mustSetState(t, svc, ctx, b, task.StateNotDone)
	if st := mustGet(t, svc, ctx, a).State; st != task.StateDone {
		t.Fatalf("dependent state = %s, want done", st)
	}
}

func TestDelete_R6_RejectsWhileBlockingOpenTask(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "open", "done", "cancelled", "blocker")
	open, done, cancelled, blocker := ids[0], ids[1], ids[2], ids[3]
	mustAddDep(t, svc, ctx, open, blocker)
	mustAddDep(t, svc, ctx, done, blocker)
	mustAddDep(t, svc, ctx, cancelled, blocker)
	mustSetState(t, svc, ctx, cancelled, task.StateCancelled)

	err := svc.Delete(ctx, blocker)
	rule, details := ruleOf(err)
	if rule != task.RuleHasOpenDependents || !slices.Equal(details["dependents"].([]int64), []int64{open, done}) {
		t.Fatalf("err = %v, details = %v", err, details)
	}
	mustGet(t, svc, ctx, blocker)

	mustSetState(t, svc, ctx, open, task.StateCancelled)
	// done is still not_done here: closing it needs the blocker done first.
	mustSetState(t, svc, ctx, blocker, task.StateDone)
	mustSetState(t, svc, ctx, done, task.StateDone)
	if err := svc.Delete(ctx, blocker); err != nil {
		t.Fatalf("Delete with finished dependents: %v", err)
	}
	edges, err := svc.ListDeps(ctx)
	if err != nil {
		t.Fatalf("ListDeps: %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("edges after delete = %v", edges)
	}
}

func TestAddDep_R7_DuplicateIsNoOp(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c")
	a, b, c := ids[0], ids[1], ids[2]
	mustAddDep(t, svc, ctx, a, b)
	mustAddDep(t, svc, ctx, a, b)
	mustAddDep(t, svc, ctx, b, c)
	mustAddDep(t, svc, ctx, a, c)

	edges, err := svc.ListDeps(ctx)
	if err != nil {
		t.Fatalf("ListDeps: %v", err)
	}
	want := []task.Edge{{TaskID: a, DependsOnID: b}, {TaskID: a, DependsOnID: c}, {TaskID: b, DependsOnID: c}}
	if !slices.Equal(edges, want) {
		t.Fatalf("edges = %v, want %v", edges, want)
	}
}

func TestRemoveDep_R8_AlwaysAllowed(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b")
	a, b := ids[0], ids[1]
	mustAddDep(t, svc, ctx, a, b)
	mustSetState(t, svc, ctx, a, task.StateCancelled)

	got, err := svc.RemoveDep(ctx, a, b)
	if err != nil {
		t.Fatalf("RemoveDep: %v", err)
	}
	if len(got.BlockedBy) != 0 {
		t.Fatalf("blocked_by = %v", got.BlockedBy)
	}
	if _, err := svc.RemoveDep(ctx, a, b); err != nil {
		t.Fatalf("RemoveDep missing edge: %v", err)
	}
}

func TestDeps_R9_UnknownTaskIsNotFound(t *testing.T) {
	svc, ctx := newService(t)
	a := mkTasks(t, svc, ctx, "a")[0]

	if _, err := svc.AddDep(ctx, a, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("AddDep unknown blocker err = %v", err)
	}
	if _, err := svc.AddDep(ctx, 999, a); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("AddDep unknown task err = %v", err)
	}
}

func TestApplyDeps_AllOrNothing(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c")
	a, b, c := ids[0], ids[1], ids[2]
	mustAddDep(t, svc, ctx, b, c)

	_, err := svc.ApplyDeps(ctx, task.DepBatch{
		Remove: []task.Edge{{TaskID: b, DependsOnID: c}},
		Add:    []task.Edge{{TaskID: a, DependsOnID: b}, {TaskID: b, DependsOnID: a}},
	})
	rule, details := ruleOf(err)
	if rule != task.RuleCycle || details["index"] != 1 {
		t.Fatalf("err = %v, details = %v", err, details)
	}
	edges, _ := svc.ListDeps(ctx)
	if want := []task.Edge{{TaskID: b, DependsOnID: c}}; !slices.Equal(edges, want) {
		t.Fatalf("edges after failed batch = %v, want %v", edges, want)
	}
}

func TestApplyDeps_SwapsDirectionInOneCall(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b")
	a, b := ids[0], ids[1]
	mustAddDep(t, svc, ctx, b, a)

	got, err := svc.ApplyDeps(ctx, task.DepBatch{
		Add:    []task.Edge{{TaskID: a, DependsOnID: b}},
		Remove: []task.Edge{{TaskID: b, DependsOnID: a}},
	})
	if err != nil {
		t.Fatalf("ApplyDeps: %v", err)
	}
	if len(got) != 2 || got[0].ID != a || !slices.Equal(got[0].BlockedBy, []int64{b}) {
		t.Fatalf("returned tasks = %+v", got)
	}
}

func TestApplyDeps_CapsOperations(t *testing.T) {
	svc, ctx := newService(t)
	_, err := svc.ApplyDeps(ctx, task.DepBatch{Add: make([]task.Edge, task.MaxDepBatch+1)})
	if err == nil || !strings.Contains(err.Error(), "must be") {
		t.Fatalf("err = %v", err)
	}
}

func graphIDs(g task.Graph) []int64 {
	out := make([]int64, 0, len(g.Tasks))
	for _, tk := range g.Tasks {
		out = append(out, tk.ID)
	}
	return out
}

func TestGraph_Diamond(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c", "d")
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]
	mustAddDep(t, svc, ctx, a, b)
	mustAddDep(t, svc, ctx, a, c)
	mustAddDep(t, svc, ctx, b, d)
	mustAddDep(t, svc, ctx, c, d)
	ab, ac, bd, cd := task.Edge{TaskID: a, DependsOnID: b}, task.Edge{TaskID: a, DependsOnID: c}, task.Edge{TaskID: b, DependsOnID: d}, task.Edge{TaskID: c, DependsOnID: d}

	cases := []struct {
		start     int64
		dir       task.Direction
		wantTasks []int64
		wantEdges []task.Edge
	}{
		{a, task.DirUp, []int64{a, b, c, d}, []task.Edge{ab, ac, bd, cd}},
		{a, task.DirDown, []int64{a}, []task.Edge{}},
		{d, task.DirDown, []int64{a, b, c, d}, []task.Edge{ab, ac, bd, cd}},
		{b, task.DirUp, []int64{b, d}, []task.Edge{bd}},
		{b, task.DirDown, []int64{a, b}, []task.Edge{ab}},
		{b, task.DirBoth, []int64{a, b, d}, []task.Edge{ab, bd}},
	}
	for _, tc := range cases {
		g, err := svc.Graph(ctx, tc.start, tc.dir)
		if err != nil {
			t.Fatalf("Graph(%d, %s): %v", tc.start, tc.dir, err)
		}
		if !slices.Equal(graphIDs(g), tc.wantTasks) || !slices.Equal(g.Edges, tc.wantEdges) {
			t.Errorf("Graph(%d, %s) = tasks %v edges %v, want %v %v", tc.start, tc.dir, graphIDs(g), g.Edges, tc.wantTasks, tc.wantEdges)
		}
	}
}

func TestGraph_Chain(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "a", "b", "c", "unrelated")
	a, b, c := ids[0], ids[1], ids[2]
	mustAddDep(t, svc, ctx, a, b)
	mustAddDep(t, svc, ctx, b, c)

	g, err := svc.Graph(ctx, b, task.DirBoth)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if !slices.Equal(graphIDs(g), []int64{a, b, c}) || len(g.Edges) != 2 {
		t.Fatalf("tasks = %v edges = %v", graphIDs(g), g.Edges)
	}
	if g, _ := svc.Graph(ctx, c, task.DirUp); !slices.Equal(graphIDs(g), []int64{c}) {
		t.Fatalf("up from chain end = %v", graphIDs(g))
	}
}

func TestList_FilterByBlocked(t *testing.T) {
	svc, ctx := newService(t)
	ids := mkTasks(t, svc, ctx, "ready", "blocked", "done-blocker", "open-blocker", "unblocked")
	ready, blocked, doneBlocker, openBlocker, unblocked := ids[0], ids[1], ids[2], ids[3], ids[4]
	mustAddDep(t, svc, ctx, blocked, openBlocker)
	mustAddDep(t, svc, ctx, unblocked, doneBlocker)
	mustSetState(t, svc, ctx, doneBlocker, task.StateDone)

	yes, no := true, false
	listIDs := func(f task.FilterSort) []int64 {
		f.Sort = task.SortCreatedAt
		f.Ascending = true
		got, err := svc.List(ctx, f)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		out := make([]int64, 0, len(got))
		for _, tk := range got {
			out = append(out, tk.ID)
		}
		return out
	}
	open := []task.State{task.StateNotDone}
	if got := listIDs(task.FilterSort{States: open, Blocked: &no}); !slices.Equal(got, []int64{ready, openBlocker, unblocked}) {
		t.Fatalf("ready = %v", got)
	}
	if got := listIDs(task.FilterSort{Blocked: &yes}); !slices.Equal(got, []int64{blocked}) {
		t.Fatalf("blocked = %v", got)
	}
}
