package tag_test

import (
	"context"
	"strings"
	"testing"

	"github.com/srliao/tt/internal/db/dbtest"
	"github.com/srliao/tt/internal/tag"
)

// newSvc spins up an in-memory store and wires a tag service around it.
func newSvc(t *testing.T) *tag.Impl {
	t.Helper()
	store := dbtest.New(t)
	return tag.New(store)
}

func TestCreateAndListSorted(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	if _, err := svc.Create(ctx, "work"); err != nil {
		t.Fatalf("Create(work): %v", err)
	}
	if _, err := svc.Create(ctx, "home"); err != nil {
		t.Fatalf("Create(home): %v", err)
	}

	got, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List length = %d, want 2 (%+v)", len(got), got)
	}
	if got[0].Name != "home" || got[1].Name != "work" {
		t.Errorf("List order = [%s, %s], want [home, work]", got[0].Name, got[1].Name)
	}
}

func TestCreateNormalizesAndRejectsEmpty(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	tg, err := svc.Create(ctx, "  Spaced  ")
	if err != nil {
		t.Fatalf("Create(spaced): %v", err)
	}
	if tg.Name != "spaced" {
		t.Errorf("normalized name = %q, want %q", tg.Name, "spaced")
	}

	if _, err := svc.Create(ctx, ""); err == nil {
		t.Errorf("Create(\"\"): expected error, got nil")
	}
	if _, err := svc.Create(ctx, "   "); err == nil {
		t.Errorf("Create(whitespace): expected error, got nil")
	}
}

func TestCreateIsCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	a, err := svc.Create(ctx, "Work")
	if err != nil {
		t.Fatalf("Create(Work): %v", err)
	}
	if a.Name != "work" {
		t.Errorf("Create(Work) name = %q, want %q", a.Name, "work")
	}

	b, err := svc.Create(ctx, "WORK")
	if err != nil {
		t.Fatalf("Create(WORK): %v", err)
	}
	if b.ID != a.ID {
		t.Errorf("Create(WORK) returned new id %d, want existing id %d", b.ID, a.ID)
	}

	got, err := svc.GetByName(ctx, "WoRk")
	if err != nil {
		t.Fatalf("GetByName(WoRk): %v", err)
	}
	if got == nil || got.ID != a.ID {
		t.Errorf("GetByName(WoRk) = %+v, want id %d", got, a.ID)
	}
}

func TestRenameLowercases(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	tg, err := svc.Create(ctx, "old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	renamed, err := svc.Rename(ctx, tg.ID, "  NewName  ")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Name != "newname" {
		t.Errorf("Rename name = %q, want %q", renamed.Name, "newname")
	}
}

func TestResolveLowercasesAndDedupesByCase(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	ids, err := svc.Resolve(ctx, []string{"Work", "WORK", "home"}, true)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("Resolve returned %d ids (%v), want 2", len(ids), ids)
	}

	got, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List length = %d, want 2 (%+v)", len(got), got)
	}
	for _, tg := range got {
		if tg.Name != strings.ToLower(tg.Name) {
			t.Errorf("stored tag name %q not lowercase", tg.Name)
		}
	}
}

func TestCreateDuplicateReturnsSameID(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	a, err := svc.Create(ctx, "dup")
	if err != nil {
		t.Fatalf("Create dup #1: %v", err)
	}
	b, err := svc.Create(ctx, "dup")
	if err != nil {
		t.Fatalf("Create dup #2: %v", err)
	}
	if a.ID != b.ID {
		t.Errorf("duplicate Create ids differ: a=%d b=%d", a.ID, b.ID)
	}

	got, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("List length after dup = %d, want 1", len(got))
	}
}

func TestRename(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	tg, err := svc.Create(ctx, "old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	renamed, err := svc.Rename(ctx, tg.ID, "new")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Name != "new" {
		t.Errorf("Rename name = %q, want %q", renamed.Name, "new")
	}
	if renamed.ID != tg.ID {
		t.Errorf("Rename id changed: was %d, now %d", tg.ID, renamed.ID)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	tg, err := svc.Create(ctx, "gone")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, tg.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List after delete = %d entries, want 0", len(got))
	}
}

func TestResolveAutoCreateDeduplicates(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	ids, err := svc.Resolve(ctx, []string{"a", "b", "a"}, true)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("Resolve returned %d ids (%v), want 2", len(ids), ids)
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("Resolve returned duplicate id %d", id)
		}
		seen[id] = true
	}

	got, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("List after Resolve = %d entries, want 2 (%+v)", len(got), got)
	}
}

func TestResolveMissingErrors(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(t)

	_, err := svc.Resolve(ctx, []string{"nope"}, false)
	if err == nil {
		t.Fatalf("Resolve(nope, false): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("Resolve error %q does not mention missing tag name", err.Error())
	}
}
