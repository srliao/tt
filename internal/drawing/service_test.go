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

func TestSetSceneRoundTripsAndListOmitsScene(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	d, err := svc.Create(ctx, "a", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	scene := json.RawMessage(`{"elements":[{"id":"x"}],"appState":{}}`)
	if _, err := svc.SetScene(ctx, d.ID, scene); err != nil {
		t.Fatalf("SetScene: %v", err)
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

func TestSetSceneRejectsNonObject(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)
	d, _ := svc.Create(ctx, "a", nil)

	for _, bad := range []string{``, `[]`, `"x"`, `{"elements":`} {
		if _, err := svc.SetScene(ctx, d.ID, json.RawMessage(bad)); err == nil {
			t.Errorf("SetScene(%q) succeeded, want error", bad)
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
