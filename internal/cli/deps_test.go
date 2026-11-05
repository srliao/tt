package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/srliao/tt/internal/task"
)

func TestDepsAddRemove(t *testing.T) {
	f := newFixture(t)
	a, b := f.createTask("A"), f.createTask("B")

	var got task.Task
	f.run("deps", "add", id(b.ID), id(a.ID)).ok(t, &got)
	if got.ID != b.ID || len(got.BlockedBy) != 1 || got.BlockedBy[0] != a.ID || !got.Blocked {
		t.Fatalf("got %+v", got)
	}

	f.run("deps", "rm", id(b.ID), id(a.ID)).ok(t, &got)
	if len(got.BlockedBy) != 0 || got.Blocked {
		t.Fatalf("got %+v", got)
	}
}

func TestDepsAdd_CycleIsConflict(t *testing.T) {
	f := newFixture(t)
	a, b := f.createTask("A"), f.createTask("B")
	f.run("deps", "add", id(b.ID), id(a.ID)).ok(t, nil)

	e := f.run("deps", "add", id(a.ID), id(b.ID)).fails(t, 3, "conflict")
	if e.Details["rule"] == nil {
		t.Fatalf("details = %v", e.Details)
	}
	f.run("deps", "add", "999", id(a.ID)).fails(t, 2, "not_found")
}

func TestDepsBatch_FromFileAndStdin(t *testing.T) {
	f := newFixture(t)
	a, b, c := f.createTask("A"), f.createTask("B"), f.createTask("C")

	doc := fmt.Sprintf(`{"add": [[%d, %d], [%d, %d]]}`, b.ID, a.ID, c.ID, b.ID)
	path := filepath.Join(t.TempDir(), "deps.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Tasks []task.Task `json:"tasks"`
	}
	f.run("deps", "batch", "-f", path).ok(t, &out)
	if len(out.Tasks) == 0 {
		t.Fatalf("no tasks returned")
	}

	f.runStdin(fmt.Sprintf(`{"remove": [[%d, %d]]}`, c.ID, b.ID), "deps", "batch").ok(t, &out)

	var g task.Graph
	f.run("deps", "graph", id(b.ID)).ok(t, &g)
	if len(g.Edges) != 1 || g.Edges[0].TaskID != b.ID || g.Edges[0].DependsOnID != a.ID {
		t.Fatalf("edges = %+v", g.Edges)
	}

	f.runStdin("not json", "deps", "batch").fails(t, 3, "validation_failed")
}

func TestDepsGraph_Direction(t *testing.T) {
	f := newFixture(t)
	a, b, c := f.createTask("A"), f.createTask("B"), f.createTask("C")
	f.run("deps", "add", id(b.ID), id(a.ID)).ok(t, nil)
	f.run("deps", "add", id(c.ID), id(b.ID)).ok(t, nil)

	var g task.Graph
	f.run("deps", "graph", id(b.ID), "--direction", "up").ok(t, &g)
	if len(g.Edges) != 1 || g.Edges[0].DependsOnID != a.ID {
		t.Fatalf("up edges = %+v", g.Edges)
	}
	f.run("deps", "graph", id(b.ID), "--direction", "sideways").fails(t, 3, "validation_failed")
	f.run("deps", "graph", "999").fails(t, 2, "not_found")
}
