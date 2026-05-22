package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/srliao/tt/internal/task"
)

func TestStage_Reorder(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	a, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "b"})
	c, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "c"})

	for _, id := range []int64{a.ID, b.ID, c.ID} {
		if _, err := fx.tasks.Stage(ctx, id); err != nil {
			t.Fatalf("stage %d: %v", id, err)
		}
	}

	resp := doJSON(t, http.MethodPost, fx.server.URL+"/api/v1/stage/reorder", map[string]any{
		"task_id":   c.ID,
		"before_id": a.ID,
		"after_id":  b.ID,
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := decodeTask(t, resp)
	if got.StagedOrder == nil {
		t.Fatalf("staged_order is nil")
	}

	// Re-fetch a and b to get the original staged_order values for comparison.
	aReloaded, err := fx.tasks.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("reload a: %v", err)
	}
	bReloaded, err := fx.tasks.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("reload b: %v", err)
	}
	if aReloaded.StagedOrder == nil || bReloaded.StagedOrder == nil {
		t.Fatalf("neighbor staged_order is nil")
	}
	if !(*got.StagedOrder > *aReloaded.StagedOrder && *got.StagedOrder < *bReloaded.StagedOrder) {
		t.Fatalf("staged_order %v not between %v and %v", *got.StagedOrder, *aReloaded.StagedOrder, *bReloaded.StagedOrder)
	}
}

func TestStage_ClearAll(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	a, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "b"})
	_, _ = fx.tasks.Stage(ctx, a.ID)
	_, _ = fx.tasks.Stage(ctx, b.ID)

	resp := doJSON(t, http.MethodDelete, fx.server.URL+"/api/v1/stage", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	aReloaded, _ := fx.tasks.Get(ctx, a.ID)
	bReloaded, _ := fx.tasks.Get(ctx, b.ID)
	if aReloaded.StagedOrder != nil || bReloaded.StagedOrder != nil {
		t.Fatalf("staged_order not cleared: %v %v", aReloaded.StagedOrder, bReloaded.StagedOrder)
	}
}

func TestStage_ClearFinished(t *testing.T) {
	t.Parallel()

	fx := newTestServer(t, nil)
	ctx := context.Background()
	a, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "a"})
	b, _ := fx.tasks.Create(ctx, task.CreateInput{Title: "b"})
	_, _ = fx.tasks.Stage(ctx, a.ID)
	_, _ = fx.tasks.Stage(ctx, b.ID)
	// Mark a as done; expect ClearFinished to drop it but keep b.
	if _, err := fx.tasks.SetState(ctx, a.ID, task.StateDone); err != nil {
		t.Fatalf("done: %v", err)
	}

	resp := doJSON(t, http.MethodDelete, fx.server.URL+"/api/v1/stage/finished", nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	aReloaded, _ := fx.tasks.Get(ctx, a.ID)
	bReloaded, _ := fx.tasks.Get(ctx, b.ID)
	if aReloaded.StagedOrder != nil {
		t.Fatalf("a (done) still staged: %v", *aReloaded.StagedOrder)
	}
	if bReloaded.StagedOrder == nil {
		t.Fatalf("b (not done) should still be staged")
	}
}
