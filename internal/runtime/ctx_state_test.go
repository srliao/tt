package runtime

import (
	"reflect"
	"testing"
)

func TestStateBuffer_Get_ReadsFromInitial(t *testing.T) {
	buf := newStateBuffer(map[string]any{"k": "v"})
	if got := buf.Get("k"); got != "v" {
		t.Fatalf("Get(k) = %v, want v", got)
	}
	if got := buf.Get("missing"); got != nil {
		t.Fatalf("Get(missing) = %v, want nil", got)
	}
}

func TestStateBuffer_Set_ObservableThroughGet(t *testing.T) {
	buf := newStateBuffer(map[string]any{"a": 1})
	buf.Set("a", 2)
	buf.Set("b", "x")
	if got := buf.Get("a"); got != 2 {
		t.Fatalf("after Set(a,2), Get(a) = %v, want 2", got)
	}
	if got := buf.Get("b"); got != "x" {
		t.Fatalf("Get(b) = %v, want x", got)
	}
}

func TestStateBuffer_Delete_HidesKey(t *testing.T) {
	buf := newStateBuffer(map[string]any{"k": "v"})
	buf.Delete("k")
	if got := buf.Get("k"); got != nil {
		t.Fatalf("after Delete(k), Get(k) = %v, want nil", got)
	}
	// And Set after Delete should revive the key.
	buf.Set("k", "back")
	if got := buf.Get("k"); got != "back" {
		t.Fatalf("after Set(k,back) post-Delete, Get(k) = %v, want back", got)
	}
}

func TestStateBuffer_All_MergedView(t *testing.T) {
	buf := newStateBuffer(map[string]any{"a": 1, "b": 2, "c": 3})
	buf.Set("b", 22)
	buf.Delete("c")
	buf.Set("d", 4)
	got := buf.All()
	want := map[string]any{"a": 1, "b": 22, "d": 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("All() = %v, want %v", got, want)
	}
}

func TestStateBuffer_FlushIsIdempotent(t *testing.T) {
	buf := newStateBuffer(map[string]any{"a": 1})
	buf.Set("b", 2)
	first := buf.Flush()
	second := buf.Flush()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Flush() not idempotent: %v vs %v", first, second)
	}
	if !reflect.DeepEqual(first, map[string]any{"a": 1, "b": 2}) {
		t.Fatalf("Flush() = %v, want {a:1, b:2}", first)
	}
}

func TestStateBuffer_DoesNotMutateInitial(t *testing.T) {
	initial := map[string]any{"a": 1}
	buf := newStateBuffer(initial)
	buf.Set("a", 99)
	buf.Set("b", 2)
	buf.Delete("a")
	if v, ok := initial["a"]; !ok || v != 1 {
		t.Fatalf("initial map mutated: a=%v ok=%v, want 1 true", v, ok)
	}
	if _, ok := initial["b"]; ok {
		t.Fatalf("initial map gained key b")
	}
}
