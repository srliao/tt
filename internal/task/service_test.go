package task_test

import (
	"context"
	"encoding/json"
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

// insertOKRun records a synthetic OK script_run with the given spawned task
// ids. Used by the lastSpawn batch tests, which rely on the run row rather
// than tasks.spawned_by_script_id to define "the latest batch".
func insertOKRun(t *testing.T, store *db.Store, ctx context.Context, scriptID int64, taskIDs []int64) {
	t.Helper()
	insertRun(t, store, ctx, scriptID, "ok", taskIDs)
}

func insertRun(t *testing.T, store *db.Store, ctx context.Context, scriptID int64, status string, taskIDs []int64) {
	t.Helper()
	ids, err := json.Marshal(taskIDs)
	if err != nil {
		t.Fatalf("marshal task ids: %v", err)
	}
	_, err = store.DB().ExecContext(ctx,
		`INSERT INTO script_runs (script_id, status, finished_at, spawned_task_ids, trigger)
		 VALUES (?, ?, datetime('now'), ?, 'manual')`,
		scriptID, status, string(ids))
	if err != nil {
		t.Fatalf("insertRun: %v", err)
	}
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

func TestLatestBySpawningScripts_ReturnsLatestBatch(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	sid := insertScript(t, store, ctx, "s1")

	// First batch: one task. Sleep across a second boundary so the second
	// run's started_at is strictly newer.
	first, err := svc.Create(ctx, task.CreateInput{Title: "old", SpawnedByScriptID: &sid})
	if err != nil {
		t.Fatalf("Create(old): %v", err)
	}
	insertOKRun(t, store, ctx, sid, []int64{first.ID})
	time.Sleep(1100 * time.Millisecond)

	// Second batch: two tasks.
	a, err := svc.Create(ctx, task.CreateInput{Title: "a", SpawnedByScriptID: &sid})
	if err != nil {
		t.Fatalf("Create(a): %v", err)
	}
	b, err := svc.Create(ctx, task.CreateInput{Title: "b", SpawnedByScriptID: &sid})
	if err != nil {
		t.Fatalf("Create(b): %v", err)
	}
	insertOKRun(t, store, ctx, sid, []int64{a.ID, b.ID})

	got, err := svc.LatestBySpawningScripts(ctx, sid)
	if err != nil {
		t.Fatalf("LatestBySpawningScripts: %v", err)
	}
	if len(got) != 2 || got[0].ID != a.ID || got[1].ID != b.ID {
		t.Fatalf("got = %+v, want [%d, %d]", got, a.ID, b.ID)
	}
}

func TestLatestBySpawningScripts_NoRunsReturnsEmpty(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)
	sid := insertScript(t, store, ctx, "empty")

	got, err := svc.LatestBySpawningScripts(ctx, sid)
	if err != nil {
		t.Fatalf("LatestBySpawningScripts: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty", got)
	}
}

func TestLatestBySpawningScripts_IgnoresFailedRuns(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)
	sid := insertScript(t, store, ctx, "s")

	good, err := svc.Create(ctx, task.CreateInput{Title: "good", SpawnedByScriptID: &sid})
	if err != nil {
		t.Fatalf("Create(good): %v", err)
	}
	insertOKRun(t, store, ctx, sid, []int64{good.ID})

	time.Sleep(1100 * time.Millisecond)
	// A later non-OK run with spawned_task_ids must NOT shadow the prior OK
	// batch. (In production no rows would ever be persisted on a non-OK run,
	// but we guard the query against legacy / inconsistent rows.)
	insertRun(t, store, ctx, sid, "error", []int64{9999})

	got, err := svc.LatestBySpawningScripts(ctx, sid)
	if err != nil {
		t.Fatalf("LatestBySpawningScripts: %v", err)
	}
	if len(got) != 1 || got[0].ID != good.ID {
		t.Fatalf("got = %+v, want [%d]", got, good.ID)
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

func TestBulkTag_AddAttachesEveryTagToEveryTask(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	t1, _ := svc.Create(ctx, task.CreateInput{Title: "t1"})
	t2, _ := svc.Create(ctx, task.CreateInput{Title: "t2"})
	t3, _ := svc.Create(ctx, task.CreateInput{Title: "t3"})
	tagA := insertTag(t, store, ctx, "alpha")
	tagB := insertTag(t, store, ctx, "bravo")

	out, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs:    []int64{t1.ID, t2.ID, t3.ID},
		Op:     task.BulkTagOpAdd,
		TagIDs: []int64{tagA, tagB},
	})
	if err != nil {
		t.Fatalf("BulkTag add: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
	for _, got := range out {
		if !equalStringSets(got.Tags, []string{"alpha", "bravo"}) {
			t.Fatalf("task %d tags = %v, want [alpha bravo]", got.ID, got.Tags)
		}
	}
}

func TestBulkTag_AddIsIdempotent(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	t1, _ := svc.Create(ctx, task.CreateInput{Title: "t1"})
	tagA := insertTag(t, store, ctx, "alpha")
	tagB := insertTag(t, store, ctx, "bravo")

	// Pre-seed task t1 with tagA so the add-loop sees an existing row.
	if err := svc.SetTagsByID(ctx, t1.ID, []int64{tagA}); err != nil {
		t.Fatalf("SetTagsByID: %v", err)
	}

	out, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs:    []int64{t1.ID},
		Op:     task.BulkTagOpAdd,
		TagIDs: []int64{tagA, tagB},
	})
	if err != nil {
		t.Fatalf("BulkTag add (idempotent): %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if !equalStringSets(out[0].Tags, []string{"alpha", "bravo"}) {
		t.Fatalf("tags = %v, want [alpha bravo]", out[0].Tags)
	}
}

func TestBulkTag_RemoveDetachesOnlySpecifiedTags(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	t1, _ := svc.Create(ctx, task.CreateInput{Title: "t1"})
	t2, _ := svc.Create(ctx, task.CreateInput{Title: "t2"})
	tagA := insertTag(t, store, ctx, "alpha")
	tagB := insertTag(t, store, ctx, "bravo")
	tagC := insertTag(t, store, ctx, "charlie")

	if err := svc.SetTagsByID(ctx, t1.ID, []int64{tagA, tagB, tagC}); err != nil {
		t.Fatalf("seed t1: %v", err)
	}
	if err := svc.SetTagsByID(ctx, t2.ID, []int64{tagB}); err != nil {
		t.Fatalf("seed t2: %v", err)
	}

	out, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs:    []int64{t1.ID, t2.ID},
		Op:     task.BulkTagOpRemove,
		TagIDs: []int64{tagA, tagB},
	})
	if err != nil {
		t.Fatalf("BulkTag remove: %v", err)
	}
	if !equalStringSets(out[0].Tags, []string{"charlie"}) {
		t.Fatalf("t1 tags = %v, want [charlie]", out[0].Tags)
	}
	if len(out[1].Tags) != 0 {
		t.Fatalf("t2 tags = %v, want []", out[1].Tags)
	}
}

func TestBulkTag_RemoveWithEmptyTagIDsIsNoOp(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	// The HTTP handler resolves tag names via ResolveExisting, which silently
	// drops unknown names. An all-unknown payload arrives here with TagIDs=nil
	// and must be a no-op: no error, tasks returned unchanged.
	t1, _ := svc.Create(ctx, task.CreateInput{Title: "t1"})
	tagA := insertTag(t, store, ctx, "alpha")
	if err := svc.SetTagsByID(ctx, t1.ID, []int64{tagA}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs:    []int64{t1.ID},
		Op:     task.BulkTagOpRemove,
		TagIDs: nil,
	})
	if err != nil {
		t.Fatalf("BulkTag remove (empty TagIDs): %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if !equalStringSets(out[0].Tags, []string{"alpha"}) {
		t.Fatalf("tags = %v, want [alpha] (unchanged)", out[0].Tags)
	}
}

func TestBulkTag_SetReplacesPreviousTags(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	t1, _ := svc.Create(ctx, task.CreateInput{Title: "t1"})
	t2, _ := svc.Create(ctx, task.CreateInput{Title: "t2"})
	tagA := insertTag(t, store, ctx, "alpha")
	tagB := insertTag(t, store, ctx, "bravo")
	tagC := insertTag(t, store, ctx, "charlie")

	if err := svc.SetTagsByID(ctx, t1.ID, []int64{tagA, tagB}); err != nil {
		t.Fatalf("seed t1: %v", err)
	}
	if err := svc.SetTagsByID(ctx, t2.ID, []int64{tagA}); err != nil {
		t.Fatalf("seed t2: %v", err)
	}

	out, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs:    []int64{t1.ID, t2.ID},
		Op:     task.BulkTagOpSet,
		TagIDs: []int64{tagC},
	})
	if err != nil {
		t.Fatalf("BulkTag set: %v", err)
	}
	for _, got := range out {
		if !equalStringSets(got.Tags, []string{"charlie"}) {
			t.Fatalf("task %d tags = %v, want [charlie]", got.ID, got.Tags)
		}
	}
}

func TestBulkTag_SetWithEmptyTagIDsClearsAll(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	t1, _ := svc.Create(ctx, task.CreateInput{Title: "t1"})
	tagA := insertTag(t, store, ctx, "alpha")
	if err := svc.SetTagsByID(ctx, t1.ID, []int64{tagA}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs:    []int64{t1.ID},
		Op:     task.BulkTagOpSet,
		TagIDs: nil,
	})
	if err != nil {
		t.Fatalf("BulkTag set (clear): %v", err)
	}
	if len(out[0].Tags) != 0 {
		t.Fatalf("tags = %v, want []", out[0].Tags)
	}
}

func TestBulkTag_ValidationErrors(t *testing.T) {
	t.Parallel()
	svc, _, ctx := newServiceWithStore(t)

	if _, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs: nil, Op: task.BulkTagOpAdd, TagIDs: []int64{1},
	}); err == nil {
		t.Fatalf("expected error for empty IDs")
	}
	if _, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs: []int64{1}, Op: task.BulkTagOp("nope"), TagIDs: []int64{1},
	}); err == nil {
		t.Fatalf("expected error for invalid Op")
	}
	if _, err := svc.BulkTag(ctx, task.BulkTagInput{
		IDs: []int64{1}, Op: task.BulkTagOpAdd, TagIDs: nil,
	}); err == nil {
		t.Fatalf("expected error for empty TagIDs on add")
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

func TestList_FilterByTags_ModeAllAndAny(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	t1, err := svc.Create(ctx, task.CreateInput{Title: "single-tag"})
	if err != nil {
		t.Fatalf("Create t1: %v", err)
	}
	t2, err := svc.Create(ctx, task.CreateInput{Title: "double-tag"})
	if err != nil {
		t.Fatalf("Create t2: %v", err)
	}

	tagA := insertTag(t, store, ctx, "alpha")
	tagB := insertTag(t, store, ctx, "bravo")

	if err := svc.SetTagsByID(ctx, t1.ID, []int64{tagA}); err != nil {
		t.Fatalf("SetTagsByID t1: %v", err)
	}
	if err := svc.SetTagsByID(ctx, t2.ID, []int64{tagA, tagB}); err != nil {
		t.Fatalf("SetTagsByID t2: %v", err)
	}

	// TagModeAll: only t2 carries both alpha and bravo.
	gotAll, err := svc.List(ctx, task.FilterSort{
		Tags: task.TagFilter{Mode: task.TagModeAll, RealTagIDs: []int64{tagA, tagB}},
	})
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if len(gotAll) != 1 || gotAll[0].ID != t2.ID {
		t.Fatalf("TagModeAll = %+v, want [%d]", gotAll, t2.ID)
	}

	// TagModeAny returns every task carrying at least one of the supplied tags.
	gotAny, err := svc.List(ctx, task.FilterSort{
		Tags: task.TagFilter{Mode: task.TagModeAny, RealTagIDs: []int64{tagA, tagB}},
	})
	if err != nil {
		t.Fatalf("List(any): %v", err)
	}
	if len(gotAny) != 2 {
		t.Fatalf("TagModeAny len = %d, want 2 (%+v)", len(gotAny), gotAny)
	}
	seen := map[int64]bool{gotAny[0].ID: true, gotAny[1].ID: true}
	if !seen[t1.ID] || !seen[t2.ID] {
		t.Fatalf("TagModeAny missing expected ids: got %+v", gotAny)
	}
}

// TestList_FilterByTags_Untagged covers the @untagged sentinel branches and
// the impossible "all + @untagged + real tags" short-circuit.
func TestList_FilterByTags_Untagged(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	bare, err := svc.Create(ctx, task.CreateInput{Title: "bare"})
	if err != nil {
		t.Fatalf("Create bare: %v", err)
	}
	workOnly, err := svc.Create(ctx, task.CreateInput{Title: "work-only"})
	if err != nil {
		t.Fatalf("Create work-only: %v", err)
	}
	homeOnly, err := svc.Create(ctx, task.CreateInput{Title: "home-only"})
	if err != nil {
		t.Fatalf("Create home-only: %v", err)
	}

	tagWork := insertTag(t, store, ctx, "work")
	tagHome := insertTag(t, store, ctx, "home")
	if err := svc.SetTagsByID(ctx, workOnly.ID, []int64{tagWork}); err != nil {
		t.Fatalf("SetTagsByID work-only: %v", err)
	}
	if err := svc.SetTagsByID(ctx, homeOnly.ID, []int64{tagHome}); err != nil {
		t.Fatalf("SetTagsByID home-only: %v", err)
	}

	// Untagged-only: just `bare`.
	gotUntag, err := svc.List(ctx, task.FilterSort{
		Tags: task.TagFilter{Mode: task.TagModeAny, IncludeUntagged: true},
	})
	if err != nil {
		t.Fatalf("List(untagged): %v", err)
	}
	if len(gotUntag) != 1 || gotUntag[0].ID != bare.ID {
		t.Fatalf("untagged-only = %+v, want [%d]", gotUntag, bare.ID)
	}

	// Any + @untagged + work: union of (untagged) ∪ (carries work).
	gotUnion, err := svc.List(ctx, task.FilterSort{
		Tags: task.TagFilter{
			Mode:            task.TagModeAny,
			RealTagIDs:      []int64{tagWork},
			IncludeUntagged: true,
		},
	})
	if err != nil {
		t.Fatalf("List(any untagged+work): %v", err)
	}
	if len(gotUnion) != 2 {
		t.Fatalf("any untagged+work len = %d (%+v)", len(gotUnion), gotUnion)
	}
	got := map[int64]bool{gotUnion[0].ID: true, gotUnion[1].ID: true}
	if !got[bare.ID] || !got[workOnly.ID] {
		t.Fatalf("any untagged+work missing ids: %+v", gotUnion)
	}

	// All + @untagged + work: impossible set, must return empty (not 500).
	gotImpossible, err := svc.List(ctx, task.FilterSort{
		Tags: task.TagFilter{
			Mode:            task.TagModeAll,
			RealTagIDs:      []int64{tagWork},
			IncludeUntagged: true,
		},
	})
	if err != nil {
		t.Fatalf("List(all untagged+work): %v", err)
	}
	if len(gotImpossible) != 0 {
		t.Fatalf("all untagged+work = %+v, want []", gotImpossible)
	}
}

func TestList_FilterByTagsExclude(t *testing.T) {
	t.Parallel()
	svc, store, ctx := newServiceWithStore(t)

	plain, err := svc.Create(ctx, task.CreateInput{Title: "plain"})
	if err != nil {
		t.Fatalf("Create plain: %v", err)
	}
	tagged, err := svc.Create(ctx, task.CreateInput{Title: "tagged"})
	if err != nil {
		t.Fatalf("Create tagged: %v", err)
	}
	both, err := svc.Create(ctx, task.CreateInput{Title: "both"})
	if err != nil {
		t.Fatalf("Create both: %v", err)
	}

	tagA := insertTag(t, store, ctx, "alpha")
	tagB := insertTag(t, store, ctx, "bravo")

	if err := svc.SetTagsByID(ctx, tagged.ID, []int64{tagA}); err != nil {
		t.Fatalf("SetTagsByID tagged: %v", err)
	}
	if err := svc.SetTagsByID(ctx, both.ID, []int64{tagA, tagB}); err != nil {
		t.Fatalf("SetTagsByID both: %v", err)
	}

	// Exclude alpha → only the plain task survives.
	got, err := svc.List(ctx, task.FilterSort{TagExcludeIDs: []int64{tagA}})
	if err != nil {
		t.Fatalf("List(exclude alpha): %v", err)
	}
	if len(got) != 1 || got[0].ID != plain.ID {
		t.Fatalf("exclude alpha = %+v, want [%d]", got, plain.ID)
	}

	// Combine inclusion (alpha) with exclusion (bravo): tagged passes,
	// both is dropped because it carries bravo.
	got, err = svc.List(ctx, task.FilterSort{
		Tags:          task.TagFilter{Mode: task.TagModeAny, RealTagIDs: []int64{tagA}},
		TagExcludeIDs: []int64{tagB},
	})
	if err != nil {
		t.Fatalf("List(include alpha exclude bravo): %v", err)
	}
	if len(got) != 1 || got[0].ID != tagged.ID {
		t.Fatalf("include alpha exclude bravo = %+v, want [%d]", got, tagged.ID)
	}
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
