package task_test

import (
	"context"
	"testing"
	"time"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/task"
)

// fakeClock is a settable clock so a test can move time between calls.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestServiceStampsTimestampsFromClock(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)}
	svc := task.New(dbtest.New(t), task.WithClock(clk.now))

	created, err := svc.Create(ctx, task.CreateInput{Title: "a"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created.CreatedAt.Equal(clk.t) || !created.UpdatedAt.Equal(clk.t) {
		t.Fatalf("create stamps = %v / %v, want %v", created.CreatedAt, created.UpdatedAt, clk.t)
	}

	clk.t = time.Date(2031, 3, 5, 0, 0, 0, 0, time.UTC)
	updated, err := svc.Update(ctx, created.ID, task.UpdateInput{Title: "b"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated.UpdatedAt.Equal(clk.t) {
		t.Errorf("update updated_at = %v, want %v", updated.UpdatedAt, clk.t)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("update changed created_at to %v", updated.CreatedAt)
	}

	clk.t = time.Date(2031, 3, 6, 0, 0, 0, 0, time.UTC)
	done, err := svc.SetState(ctx, created.ID, task.StateDone)
	if err != nil {
		t.Fatalf("SetState done: %v", err)
	}
	if done.CompletedAt == nil || !done.CompletedAt.Equal(clk.t) {
		t.Errorf("completed_at = %v, want %v", done.CompletedAt, clk.t)
	}
	if !done.UpdatedAt.Equal(clk.t) {
		t.Errorf("state updated_at = %v, want %v", done.UpdatedAt, clk.t)
	}

	clk.t = time.Date(2031, 3, 7, 0, 0, 0, 0, time.UTC)
	cancelled, err := svc.SetState(ctx, created.ID, task.StateCancelled)
	if err != nil {
		t.Fatalf("SetState cancelled: %v", err)
	}
	if cancelled.CancelledAt == nil || !cancelled.CancelledAt.Equal(clk.t) {
		t.Errorf("cancelled_at = %v, want %v", cancelled.CancelledAt, clk.t)
	}

	clk.t = time.Date(2031, 3, 8, 0, 0, 0, 0, time.UTC)
	staged, err := svc.Stage(ctx, created.ID)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if !staged.UpdatedAt.Equal(clk.t) {
		t.Errorf("stage updated_at = %v, want %v", staged.UpdatedAt, clk.t)
	}
}
