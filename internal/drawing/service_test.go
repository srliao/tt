package drawing_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/drawing"
)

func newSvc(t *testing.T) *drawing.Impl {
	t.Helper()
	return drawing.New(dbtest.New(t))
}

func TestCreateStartsWithEmptyScene(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	d, err := svc.Create(ctx, "  Architecture  ", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.Name != "Architecture" {
		t.Errorf("Name = %q, want trimmed %q", d.Name, "Architecture")
	}
	if string(d.Scene) != "{}" {
		t.Errorf("Scene = %s, want {}", d.Scene)
	}
	if _, err := svc.Create(ctx, "   ", nil); err == nil {
		t.Error("Create(blank) succeeded, want name is required")
	}
}

func TestUpdateSceneRoundTripsAndListOmitsScene(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	d, err := svc.Create(ctx, "a", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	scene := json.RawMessage(`{"elements":[{"id":"x"}],"appState":{}}`)
	if _, err := svc.Update(ctx, d.ID, drawing.UpdateInput{Scene: scene}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := svc.Get(ctx, d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Scene) != string(scene) {
		t.Errorf("Scene = %s, want %s", got.Scene, scene)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != d.ID {
		t.Fatalf("List = %+v, want the one drawing", list)
	}
}

func TestUpdateRejectsNonObjectScene(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	d, _ := svc.Create(ctx, "a", nil)

	for _, bad := range []string{``, `[]`, `"x"`, `{"elements":`} {
		if _, err := svc.Update(ctx, d.ID, drawing.UpdateInput{Scene: json.RawMessage(bad)}); err == nil {
			t.Errorf("Update(scene=%q) succeeded, want error", bad)
		}
	}
}

func TestGetMissingIsNoRows(t *testing.T) {
	_, err := newSvc(t).Get(context.Background(), 999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Get(missing) err = %v, want sql.ErrNoRows", err)
	}
}

func TestCreateWithSceneStoresIt(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	scene := json.RawMessage(` {"elements":[{"id":"imported"}]} `)
	d, err := svc.Create(ctx, "Imported", scene)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if string(d.Scene) != `{"elements":[{"id":"imported"}]}` {
		t.Errorf("Scene = %s, want the imported scene trimmed", d.Scene)
	}
	if _, err := svc.Create(ctx, "Bad", json.RawMessage(`[]`)); err == nil {
		t.Error("Create with array scene succeeded, want error")
	}
	if list, _ := svc.List(ctx); len(list) != 1 {
		t.Errorf("List len = %d, want 1 (rejected create must not insert)", len(list))
	}
}

func TestUpdateIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	d, _ := svc.Create(ctx, "before", nil)

	name := "after"
	if _, err := svc.Update(ctx, d.ID, drawing.UpdateInput{Name: &name, Scene: json.RawMessage(`[]`)}); err == nil {
		t.Fatal("Update with bad scene succeeded, want error")
	}
	got, _ := svc.Get(ctx, d.ID)
	if got.Name != "before" {
		t.Errorf("Name = %q after rejected update, want unchanged %q", got.Name, "before")
	}

	scene := json.RawMessage(`{"elements":[]}`)
	got, err := svc.Update(ctx, d.ID, drawing.UpdateInput{Name: &name, Scene: scene})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "after" || string(got.Scene) != string(scene) {
		t.Errorf("got name=%q scene=%s, want both applied", got.Name, got.Scene)
	}

	if _, err := svc.Update(ctx, 999, drawing.UpdateInput{Name: &name}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Update(missing) err = %v, want sql.ErrNoRows", err)
	}
}
