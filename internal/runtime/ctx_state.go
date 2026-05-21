package runtime

import (
	"fmt"

	"github.com/dop251/goja"
)

// stateBuffer holds the in-memory overlay applied to a script's persisted
// user_state for the duration of a single run. Writes and deletes are buffered
// here so they only become visible after a successful run (spec §5: state
// flushes on RunStatusOK only).
//
// The buffer never mutates the supplied initial map — callers can keep using
// their snapshot independently. The cost is one shallow copy per Flush() /
// All(), which is fine given user_state is a JSON document, not a hot path.
type stateBuffer struct {
	initial map[string]any
	writes  map[string]any
	deletes map[string]struct{}
}

// newStateBuffer wraps initial in a fresh overlay. A nil initial is treated as
// an empty starting state.
func newStateBuffer(initial map[string]any) *stateBuffer {
	if initial == nil {
		initial = map[string]any{}
	}
	return &stateBuffer{
		initial: initial,
		writes:  map[string]any{},
		deletes: map[string]struct{}{},
	}
}

// Get returns the effective value of k: pending writes first, then initial,
// returning nil for keys that have been deleted or never existed.
func (b *stateBuffer) Get(k string) any {
	if _, gone := b.deletes[k]; gone {
		return nil
	}
	if v, ok := b.writes[k]; ok {
		return v
	}
	if v, ok := b.initial[k]; ok {
		return v
	}
	return nil
}

// Set records a pending write of k=v. A prior Delete on the same key is
// rescinded so the new value is observable through Get.
func (b *stateBuffer) Set(k string, v any) {
	b.writes[k] = v
	delete(b.deletes, k)
}

// Delete schedules removal of k. A pending write on the same key is dropped
// because the delete supersedes it.
func (b *stateBuffer) Delete(k string) {
	delete(b.writes, k)
	b.deletes[k] = struct{}{}
}

// All returns a snapshot of the merged view (initial overlaid with pending
// writes/deletes). The returned map is a fresh allocation so callers can
// safely mutate it.
func (b *stateBuffer) All() map[string]any {
	out := make(map[string]any, len(b.initial)+len(b.writes))
	for k, v := range b.initial {
		if _, gone := b.deletes[k]; gone {
			continue
		}
		out[k] = v
	}
	for k, v := range b.writes {
		out[k] = v
	}
	return out
}

// Flush returns the JSON-serializable view of the buffer. It is idempotent —
// repeated calls yield equal snapshots and do not mutate buffer state — so
// the caller can compute the persisted blob before and after the kernel's
// success branch without surprises.
func (b *stateBuffer) Flush() map[string]any {
	return b.All()
}

// stateBindings returns the {get, set, delete, all} function set the runtime
// installs under ctx.state. Each binding lives in plain Go and gains JS
// access via the goja auto-marshaller.
func stateBindings(buf *stateBuffer) map[string]any {
	return map[string]any{
		"get": func(k string) any {
			return buf.Get(k)
		},
		"set": func(k string, v goja.Value) {
			var exported any
			if v != nil {
				exported = v.Export()
			}
			buf.Set(k, exported)
		},
		"delete": func(k string) {
			buf.Delete(k)
		},
		"all": func() map[string]any {
			return buf.All()
		},
	}
}

// installState wires stateBindings into ctx.state on rt. The state object
// itself is constructed lazily here so callers don't need to know about goja
// internals.
func installState(rt *goja.Runtime, ctxObj *goja.Object, buf *stateBuffer) error {
	stateObj := rt.NewObject()
	for name, fn := range stateBindings(buf) {
		if err := stateObj.Set(name, fn); err != nil {
			return fmt.Errorf("runtime: set ctx.state.%s: %w", name, err)
		}
	}
	if err := ctxObj.Set("state", stateObj); err != nil {
		return fmt.Errorf("runtime: set ctx.state: %w", err)
	}
	return nil
}
