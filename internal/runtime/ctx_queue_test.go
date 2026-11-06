package runtime

import (
	"reflect"
	"testing"
)

func TestTaskQueue_Enqueue_AddsOne(t *testing.T) {
	q := newTaskQueue()
	if err := q.Enqueue(map[string]any{"title": "x"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	items := q.Drain()
	if len(items) != 1 {
		t.Fatalf("Drain() len = %d, want 1", len(items))
	}
	if items[0].Title != "x" {
		t.Fatalf("title = %q, want x", items[0].Title)
	}
}

func TestTaskQueue_Enqueue_EmptyTitleRejected(t *testing.T) {
	q := newTaskQueue()
	if err := q.Enqueue(map[string]any{"title": "   "}); err == nil {
		t.Fatalf("Enqueue with blank title: want error, got nil")
	}
	if err := q.Enqueue(map[string]any{}); err == nil {
		t.Fatalf("Enqueue with missing title: want error, got nil")
	}
	if len(q.Drain()) != 0 {
		t.Fatalf("rejected entries should not be queued")
	}
}

func TestTaskQueue_Enqueue_InvalidDueDateRejected(t *testing.T) {
	q := newTaskQueue()
	if err := q.Enqueue(map[string]any{"title": "x", "due_date": "not-a-date"}); err == nil {
		t.Fatalf("Enqueue invalid due_date: want error, got nil")
	}
}

func TestTaskQueue_Enqueue_NormalizesTags(t *testing.T) {
	q := newTaskQueue()
	if err := q.Enqueue(map[string]any{
		"title": "x",
		"tags":  []any{"foo", "  foo  ", "bar", "", "bar"},
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got := q.Drain()
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	want := []string{"foo", "bar"}
	if !reflect.DeepEqual(got[0].Tags, want) {
		t.Fatalf("tags = %v, want %v", got[0].Tags, want)
	}
}

func TestTaskQueue_DrainEmptiesQueue(t *testing.T) {
	q := newTaskQueue()
	if err := q.Enqueue(map[string]any{"title": "x"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if got := len(q.Drain()); got != 1 {
		t.Fatalf("first drain len = %d, want 1", got)
	}
	if got := len(q.Drain()); got != 0 {
		t.Fatalf("second drain len = %d, want 0", got)
	}
}

func TestTaskQueue_Enqueue_NormalizesProject(t *testing.T) {
	q := newTaskQueue()
	if err := q.Enqueue(map[string]any{"title": "x", "project": "  Home "}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q.Enqueue(map[string]any{"title": "y", "project": ""}); err != nil {
		t.Fatalf("Enqueue blank project: %v", err)
	}
	got := q.Drain()
	if got[0].Project == nil || *got[0].Project != "home" {
		t.Fatalf("project = %v, want home", got[0].Project)
	}
	if got[1].Project != nil {
		t.Fatalf("blank project = %q, want nil", *got[1].Project)
	}
}

func TestTaskQueue_Enqueue_InvalidProjectRejected(t *testing.T) {
	q := newTaskQueue()
	for _, p := range []any{"@x", "a,b", 42} {
		if err := q.Enqueue(map[string]any{"title": "x", "project": p}); err == nil {
			t.Fatalf("Enqueue project %v: want error, got nil", p)
		}
	}
	if len(q.Drain()) != 0 {
		t.Fatalf("rejected entries should not be queued")
	}
}
