package drawing_test

import (
	"context"
	"testing"
	"time"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/drawing"
)

func TestServiceStampsTimestampsFromClock(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	svc := drawing.New(dbtest.New(t), drawing.WithClock(func() time.Time { return at }))

	d, err := svc.Create(ctx, "d", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !d.CreatedAt.Equal(at) || !d.UpdatedAt.Equal(at) {
		t.Fatalf("create stamps = %v / %v, want %v", d.CreatedAt, d.UpdatedAt, at)
	}

	at = time.Date(2031, 3, 5, 0, 0, 0, 0, time.UTC)
	name := "renamed"
	up, err := svc.Update(ctx, d.ID, drawing.UpdateInput{Name: &name})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !up.UpdatedAt.Equal(at) {
		t.Errorf("update updated_at = %v, want %v", up.UpdatedAt, at)
	}
}
