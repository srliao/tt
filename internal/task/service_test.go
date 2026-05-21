package task_test

import (
	"context"
	"testing"
	"time"

	"github.com/srliao/tt/internal/db"
	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/task"
)

func newService(t *testing.T) (*task.Impl, context.Context) {
	t.Helper()
	store := dbtest.New(t)
	return task.New(store), context.Background()
}

func newServiceWithStore(t *testing.T) (*task.Impl, *db.Store, context.Context) {
	t.Helper()
	store := dbtest.New(t)
	return task.New(store), store, context.Background()
}

// insertScript inserts a script directly via raw SQL so task tests can use a
// spawned_by_script_id that satisfies the FK before the scripts service
// exists.
func insertScript(t *testing.T, store *db.Store, ctx context.Context, name string) int64 {
	t.Helper()
	res, err := store.DB().ExecContext(ctx,
		`INSERT INTO scripts (name, code, schedule_kind) VALUES (?, ?, 'every_tick')`, name, "")
	if err != nil {
		t.Fatalf("insertScript: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insertScript LastInsertId: %v", err)
	}
	return id
}

// insertTag inserts a tag directly via raw SQL for tag-attachment tests.
func insertTag(t *testing.T, store *db.Store, ctx context.Context, name string) int64 {
	t.Helper()
	res, err := store.DB().ExecContext(ctx,
		`INSERT INTO tags (name) VALUES (?)`, name)
	if err != nil {
		t.Fatalf("insertTag: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insertTag LastInsertId: %v", err)
	}
	return id
}

func TestCreate_AssignsAscendingPriority(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, err := svc.Create(ctx, task.CreateInput{Title: "first"})
	if err != nil {
		t.Fatalf("Create(first): %v", err)
	}
	b, err := svc.Create(ctx, task.CreateInput{Title: "second"})
	if err != nil {
		t.Fatalf("Create(second): %v", err)
	}
	if !(b.Priority > a.Priority) {
		t.Fatalf("expected b.Priority (%v) > a.Priority (%v)", b.Priority, a.Priority)
	}
}

func TestCreate_DefaultsToNotDone(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	got, err := svc.Create(ctx, task.CreateInput{Title: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.State != task.StateNotDone {
		t.Fatalf("State = %q, want %q", got.State, task.StateNotDone)
	}
}

func TestCreate_StagedOrderIsNil(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	got, err := svc.Create(ctx, task.CreateInput{Title: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.StagedOrder != nil {
		t.Fatalf("StagedOrder = %v, want nil", *got.StagedOrder)
	}
}

func TestCreate_EmptyTitleErrors(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	if _, err := svc.Create(ctx, task.CreateInput{Title: "   "}); err == nil {
		t.Fatalf("Create(empty title): expected error, got nil")
	}
}

func TestCreate_InvalidDueDateErrors(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	bad := "tomorrow"
	if _, err := svc.Create(ctx, task.CreateInput{Title: "x", DueDate: &bad}); err == nil {
		t.Fatalf("Create(bad due_date): expected error, got nil")
	}
}

func TestSetState_DoneSetsCompletedAt(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "t"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.SetState(ctx, created.ID, task.StateDone)
	if err != nil {
		t.Fatalf("SetState(done): %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatalf("CompletedAt = nil, want non-nil")
	}
	if got.State != task.StateDone {
		t.Fatalf("State = %q, want %q", got.State, task.StateDone)
	}
}

func TestSetState_LeavingDoneClearsCompletedAt(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "t"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SetState(ctx, created.ID, task.StateDone); err != nil {
		t.Fatalf("SetState(done): %v", err)
	}
	got, err := svc.SetState(ctx, created.ID, task.StateNotDone)
	if err != nil {
		t.Fatalf("SetState(not_done): %v", err)
	}
	if got.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil", got.CompletedAt)
	}
}

func TestSetState_CancelledSetsCancelledAt(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "t"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.SetState(ctx, created.ID, task.StateCancelled)
	if err != nil {
		t.Fatalf("SetState(cancelled): %v", err)
	}
	if got.CancelledAt == nil {
		t.Fatalf("CancelledAt = nil, want non-nil")
	}
	got, err = svc.SetState(ctx, created.ID, task.StateNotDone)
	if err != nil {
		t.Fatalf("SetState(not_done): %v", err)
	}
	if got.CancelledAt != nil {
		t.Fatalf("CancelledAt = %v, want nil after revert", got.CancelledAt)
	}
}

// State transitions must never touch staged_order: a focused task that the
// user has staged remains in the stage when it transitions to done so they
// can see their progress through the batch.
func TestSetState_DoesNotTouchStagedOrder(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "t"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	staged, err := svc.Stage(ctx, created.ID)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if staged.StagedOrder == nil {
		t.Fatalf("staged.StagedOrder = nil, want non-nil")
	}
	want := *staged.StagedOrder

	done, err := svc.SetState(ctx, created.ID, task.StateDone)
	if err != nil {
		t.Fatalf("SetState(done): %v", err)
	}
	if done.StagedOrder == nil {
		t.Fatalf("StagedOrder = nil after SetState(done), want %v", want)
	}
	if *done.StagedOrder != want {
		t.Fatalf("StagedOrder = %v after SetState(done), want %v", *done.StagedOrder, want)
	}
}

func TestStage_AssignsAscendingStagedOrder(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, err := svc.Create(ctx, task.CreateInput{Title: "a"})
	if err != nil {
		t.Fatalf("Create(a): %v", err)
	}
	b, err := svc.Create(ctx, task.CreateInput{Title: "b"})
	if err != nil {
		t.Fatalf("Create(b): %v", err)
	}
	sa, err := svc.Stage(ctx, a.ID)
	if err != nil {
		t.Fatalf("Stage(a): %v", err)
	}
	sb, err := svc.Stage(ctx, b.ID)
	if err != nil {
		t.Fatalf("Stage(b): %v", err)
	}
	if sa.StagedOrder == nil || sb.StagedOrder == nil {
		t.Fatalf("StagedOrder nil: a=%v b=%v", sa.StagedOrder, sb.StagedOrder)
	}
	if !(*sb.StagedOrder > *sa.StagedOrder) {
		t.Fatalf("expected sb.StagedOrder (%v) > sa.StagedOrder (%v)", *sb.StagedOrder, *sa.StagedOrder)
	}
}

func TestUnstage_ClearsStagedOrder(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, err := svc.Create(ctx, task.CreateInput{Title: "a"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Stage(ctx, a.ID); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	got, err := svc.Unstage(ctx, a.ID)
	if err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	if got.StagedOrder != nil {
		t.Fatalf("StagedOrder = %v, want nil", *got.StagedOrder)
	}
}

func TestClearStage_ClearsAllRegardlessOfState(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})
	if _, err := svc.Stage(ctx, a.ID); err != nil {
		t.Fatalf("Stage(a): %v", err)
	}
	if _, err := svc.Stage(ctx, b.ID); err != nil {
		t.Fatalf("Stage(b): %v", err)
	}
	if _, err := svc.SetState(ctx, a.ID, task.StateDone); err != nil {
		t.Fatalf("SetState(done): %v", err)
	}

	if err := svc.ClearStage(ctx); err != nil {
		t.Fatalf("ClearStage: %v", err)
	}

	for _, id := range []int64{a.ID, b.ID} {
		got, err := svc.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%d): %v", id, err)
		}
		if got.StagedOrder != nil {
			t.Fatalf("task %d StagedOrder = %v, want nil", id, *got.StagedOrder)
		}
	}
}

func TestClearFinishedFromStage_OnlyDoneOrCancelled(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	active, _ := svc.Create(ctx, task.CreateInput{Title: "active"})
	done, _ := svc.Create(ctx, task.CreateInput{Title: "done"})
	cancelled, _ := svc.Create(ctx, task.CreateInput{Title: "cancelled"})

	for _, id := range []int64{active.ID, done.ID, cancelled.ID} {
		if _, err := svc.Stage(ctx, id); err != nil {
			t.Fatalf("Stage(%d): %v", id, err)
		}
	}
	if _, err := svc.SetState(ctx, done.ID, task.StateDone); err != nil {
		t.Fatalf("SetState(done): %v", err)
	}
	if _, err := svc.SetState(ctx, cancelled.ID, task.StateCancelled); err != nil {
		t.Fatalf("SetState(cancelled): %v", err)
	}

	if err := svc.ClearFinishedFromStage(ctx); err != nil {
		t.Fatalf("ClearFinishedFromStage: %v", err)
	}

	gotActive, err := svc.Get(ctx, active.ID)
	if err != nil {
		t.Fatalf("Get(active): %v", err)
	}
	if gotActive.StagedOrder == nil {
		t.Fatalf("active.StagedOrder = nil, want still staged")
	}

	for _, id := range []int64{done.ID, cancelled.ID} {
		got, err := svc.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%d): %v", id, err)
		}
		if got.StagedOrder != nil {
			t.Fatalf("task %d StagedOrder = %v, want nil", id, *got.StagedOrder)
		}
	}
}

func TestByScript_OnlyReturnsSpawned(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	sid := insertScript(t, store, ctx, "s1")
	spawned, err := svc.Create(ctx, task.CreateInput{Title: "spawned", SpawnedByScriptID: &sid})
	if err != nil {
		t.Fatalf("Create(spawned): %v", err)
	}
	if _, err := svc.Create(ctx, task.CreateInput{Title: "manual"}); err != nil {
		t.Fatalf("Create(manual): %v", err)
	}

	got, err := svc.ByScript(ctx, sid, 10, 0)
	if err != nil {
		t.Fatalf("ByScript: %v", err)
	}
	if len(got) != 1 || got[0].ID != spawned.ID {
		t.Fatalf("got = %+v, want one task with id %d", got, spawned.ID)
	}
}

func TestLatestBySpawningScript_ReturnsNewest(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	sid := insertScript(t, store, ctx, "s1")
	older, err := svc.Create(ctx, task.CreateInput{Title: "older", SpawnedByScriptID: &sid})
	if err != nil {
		t.Fatalf("Create(older): %v", err)
	}
	// SQLite datetime('now') is second-precision; sleep across a second
	// boundary so created_at differs.
	time.Sleep(1100 * time.Millisecond)
	newer, err := svc.Create(ctx, task.CreateInput{Title: "newer", SpawnedByScriptID: &sid})
	if err != nil {
		t.Fatalf("Create(newer): %v", err)
	}
	_ = older

	got, err := svc.LatestBySpawningScript(ctx, sid)
	if err != nil {
		t.Fatalf("LatestBySpawningScript: %v", err)
	}
	if got == nil || got.ID != newer.ID {
		t.Fatalf("got = %+v, want id %d", got, newer.ID)
	}
}

func TestLatestBySpawningScript_NoMatchReturnsNilNil(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)
	sid := insertScript(t, store, ctx, "empty")

	got, err := svc.LatestBySpawningScript(ctx, sid)
	if err != nil {
		t.Fatalf("LatestBySpawningScript: %v", err)
	}
	if got != nil {
		t.Fatalf("got = %+v, want nil", got)
	}
}

func TestSetTagsByID_ReplacesPreviousSet(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "t"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tagA := insertTag(t, store, ctx, "alpha")
	tagB := insertTag(t, store, ctx, "bravo")
	tagC := insertTag(t, store, ctx, "charlie")

	if err := svc.SetTagsByID(ctx, created.ID, []int64{tagA, tagB}); err != nil {
		t.Fatalf("SetTagsByID(A,B): %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !equalStringSets(got.Tags, []string{"alpha", "bravo"}) {
		t.Fatalf("Tags = %v, want [alpha bravo]", got.Tags)
	}

	if err := svc.SetTagsByID(ctx, created.ID, []int64{tagC}); err != nil {
		t.Fatalf("SetTagsByID(C): %v", err)
	}
	got, err = svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get(2): %v", err)
	}
	if !equalStringSets(got.Tags, []string{"charlie"}) {
		t.Fatalf("Tags = %v, want [charlie]", got.Tags)
	}
}

func equalStringSets(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	have := make(map[string]struct{}, len(got))
	for _, v := range got {
		have[v] = struct{}{}
	}
	for _, v := range want {
		if _, ok := have[v]; !ok {
			return false
		}
	}
	return true
}

func TestList_DefaultSortByPriority(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})
	c, _ := svc.Create(ctx, task.CreateInput{Title: "c"})

	got, err := svc.List(ctx, task.FilterSort{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantIDs := []int64{a.ID, b.ID, c.ID}
	for i, tk := range got {
		if tk.ID != wantIDs[i] {
			t.Fatalf("got[%d].ID = %d, want %d", i, tk.ID, wantIDs[i])
		}
	}
}

func TestList_FilterByState(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	keep, _ := svc.Create(ctx, task.CreateInput{Title: "keep"})
	skip, _ := svc.Create(ctx, task.CreateInput{Title: "skip"})
	if _, err := svc.SetState(ctx, skip.ID, task.StateDone); err != nil {
		t.Fatalf("SetState(done): %v", err)
	}

	got, err := svc.List(ctx, task.FilterSort{States: []task.State{task.StateNotDone}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != keep.ID {
		t.Fatalf("got = %+v, want one task with id %d", got, keep.ID)
	}
}

func TestList_SearchIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	hit, _ := svc.Create(ctx, task.CreateInput{Title: "grocery", Notes: "buy milk"})
	if _, err := svc.Create(ctx, task.CreateInput{Title: "other"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.List(ctx, task.FilterSort{Search: "MILK"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != hit.ID {
		t.Fatalf("got = %+v, want one match for id %d", got, hit.ID)
	}
}

func TestList_SortByTitle(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	c, _ := svc.Create(ctx, task.CreateInput{Title: "charlie"})
	a, _ := svc.Create(ctx, task.CreateInput{Title: "alpha"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "bravo"})

	got, err := svc.List(ctx, task.FilterSort{Sort: task.SortTitle, Ascending: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantIDs := []int64{a.ID, b.ID, c.ID}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, tk := range got {
		if tk.ID != wantIDs[i] {
			t.Fatalf("got[%d].ID = %d, want %d", i, tk.ID, wantIDs[i])
		}
	}
}

func TestList_FilterDueToday(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	today := time.Now().Format("2006-01-02")
	due, _ := svc.Create(ctx, task.CreateInput{Title: "today", DueDate: &today})
	other := "2099-01-01"
	if _, err := svc.Create(ctx, task.CreateInput{Title: "later", DueDate: &other}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, task.CreateInput{Title: "nodate"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.List(ctx, task.FilterSort{Due: task.DueToday})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != due.ID {
		t.Fatalf("got = %+v, want one match for id %d", got, due.ID)
	}
}

func TestList_OffsetWithoutLimit(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, task.CreateInput{Title: "t"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	got, err := svc.List(ctx, task.FilterSort{Offset: 1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (offset=1, no limit)", len(got))
	}
}

func TestRebalancePriority_AssignsIntegerKeys(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	ids := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		c, err := svc.Create(ctx, task.CreateInput{Title: "t"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		ids = append(ids, c.ID)
	}
	if err := svc.RebalancePriority(ctx); err != nil {
		t.Fatalf("RebalancePriority: %v", err)
	}
	// Re-fetch each task in creation order; priorities should now be 0..4.
	for i, id := range ids {
		got, err := svc.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%d): %v", id, err)
		}
		if got.Priority != float64(i) {
			t.Fatalf("task %d Priority = %v, want %v", id, got.Priority, float64(i))
		}
	}
}

func TestRebalanceStage_AssignsIntegerKeys(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	ids := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		c, err := svc.Create(ctx, task.CreateInput{Title: "t"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := svc.Stage(ctx, c.ID); err != nil {
			t.Fatalf("Stage: %v", err)
		}
		ids = append(ids, c.ID)
	}
	if err := svc.RebalanceStage(ctx); err != nil {
		t.Fatalf("RebalanceStage: %v", err)
	}
	// Re-fetch each task to verify the staged_order keys are 0..4 in the
	// original stage order (creation order).
	staged := make([]task.Task, 0, len(ids))
	for _, id := range ids {
		got, err := svc.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%d): %v", id, err)
		}
		staged = append(staged, got)
	}
	for i, tk := range staged {
		if tk.StagedOrder == nil || *tk.StagedOrder != float64(i) {
			t.Fatalf("staged[%d].StagedOrder = %v, want %v", i, tk.StagedOrder, float64(i))
		}
	}
}

func TestReorderMain_BetweenNeighbors(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})
	c, _ := svc.Create(ctx, task.CreateInput{Title: "c"})

	// Move c between a and b.
	got, err := svc.ReorderMain(ctx, c.ID, &a.ID, &b.ID)
	if err != nil {
		t.Fatalf("ReorderMain: %v", err)
	}
	if !(got.Priority > a.Priority && got.Priority < b.Priority) {
		t.Fatalf("Priority %v not strictly between %v and %v", got.Priority, a.Priority, b.Priority)
	}
}

func TestReorderMain_ToTop(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})

	got, err := svc.ReorderMain(ctx, b.ID, nil, &a.ID)
	if err != nil {
		t.Fatalf("ReorderMain to top: %v", err)
	}
	if !(got.Priority < a.Priority) {
		t.Fatalf("Priority %v not less than %v", got.Priority, a.Priority)
	}
}

func TestReorderMain_ToBottom(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})

	got, err := svc.ReorderMain(ctx, a.ID, &b.ID, nil)
	if err != nil {
		t.Fatalf("ReorderMain to bottom: %v", err)
	}
	if !(got.Priority > b.Priority) {
		t.Fatalf("Priority %v not greater than %v", got.Priority, b.Priority)
	}
}

func TestReorderStage_BetweenNeighbors(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})
	c, _ := svc.Create(ctx, task.CreateInput{Title: "c"})
	sa, err := svc.Stage(ctx, a.ID)
	if err != nil {
		t.Fatalf("Stage(a): %v", err)
	}
	sb, err := svc.Stage(ctx, b.ID)
	if err != nil {
		t.Fatalf("Stage(b): %v", err)
	}
	if _, err := svc.Stage(ctx, c.ID); err != nil {
		t.Fatalf("Stage(c): %v", err)
	}

	got, err := svc.ReorderStage(ctx, c.ID, &a.ID, &b.ID)
	if err != nil {
		t.Fatalf("ReorderStage: %v", err)
	}
	if got.StagedOrder == nil {
		t.Fatalf("StagedOrder = nil")
	}
	if !(*got.StagedOrder > *sa.StagedOrder && *got.StagedOrder < *sb.StagedOrder) {
		t.Fatalf("StagedOrder %v not strictly between %v and %v", *got.StagedOrder, *sa.StagedOrder, *sb.StagedOrder)
	}
}

func TestReorderStage_ToTop(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})
	sa, _ := svc.Stage(ctx, a.ID)
	if _, err := svc.Stage(ctx, b.ID); err != nil {
		t.Fatalf("Stage(b): %v", err)
	}

	got, err := svc.ReorderStage(ctx, b.ID, nil, &a.ID)
	if err != nil {
		t.Fatalf("ReorderStage to top: %v", err)
	}
	if got.StagedOrder == nil || !(*got.StagedOrder < *sa.StagedOrder) {
		t.Fatalf("StagedOrder %v not less than %v", got.StagedOrder, *sa.StagedOrder)
	}
}

func TestReorderStage_ToBottom(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	a, _ := svc.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := svc.Create(ctx, task.CreateInput{Title: "b"})
	if _, err := svc.Stage(ctx, a.ID); err != nil {
		t.Fatalf("Stage(a): %v", err)
	}
	sb, _ := svc.Stage(ctx, b.ID)

	got, err := svc.ReorderStage(ctx, a.ID, &b.ID, nil)
	if err != nil {
		t.Fatalf("ReorderStage to bottom: %v", err)
	}
	if got.StagedOrder == nil || !(*got.StagedOrder > *sb.StagedOrder) {
		t.Fatalf("StagedOrder %v not greater than %v", got.StagedOrder, *sb.StagedOrder)
	}
}

func TestGet_ReturnsPersistedTask(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "hi", Notes: "n"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "hi" || got.Notes != "n" {
		t.Fatalf("Get returned %+v, want title=hi notes=n", got)
	}
}

func TestUpdate_ReplacesFields(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "old"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	due := "2026-12-31"
	updated, err := svc.Update(ctx, created.ID, task.UpdateInput{
		Title:   "new",
		Notes:   "n2",
		DueDate: &due,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "new" || updated.Notes != "n2" {
		t.Fatalf("Update returned %+v, want title=new notes=n2", updated)
	}
	if updated.DueDate == nil || *updated.DueDate != due {
		t.Fatalf("DueDate = %v, want %v", updated.DueDate, due)
	}
}

func TestUpdate_EmptyTitleErrors(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, task.CreateInput{Title: "ok"})
	if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Title: ""}); err == nil {
		t.Fatalf("Update(empty title): expected error")
	}
}

func TestUpdate_InvalidDueDateErrors(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, task.CreateInput{Title: "ok"})
	bad := "yesterday"
	if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Title: "ok", DueDate: &bad}); err == nil {
		t.Fatalf("Update(bad due_date): expected error")
	}
}

func TestDelete_RemovesRow(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, _ := svc.Create(ctx, task.CreateInput{Title: "doomed"})
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, created.ID); err == nil {
		t.Fatalf("Get after Delete: expected error, got nil")
	}
}

func TestSetState_InvalidStateErrors(t *testing.T) {
	t.Parallel()
	svc, ctx := newService(t)

	created, err := svc.Create(ctx, task.CreateInput{Title: "t"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SetState(ctx, created.ID, task.State("bogus")); err == nil {
		t.Fatalf("SetState(bogus): expected error, got nil")
	}
}
