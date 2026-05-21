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
