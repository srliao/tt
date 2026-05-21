package task_test

import (
	"context"
	"testing"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/task"
)

func newService(t *testing.T) (*task.Impl, context.Context) {
	t.Helper()
	store := dbtest.New(t)
	return task.New(store), context.Background()
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
