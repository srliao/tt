package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// queuedTask captures the validated shape of a single ctx.queueTask call.
// Persistence (tag resolution + task.Create) happens only after the run ends
// in `ok` (spec §5: queued tasks are deferred until the OK outcome).
type queuedTask struct {
	Title   string
	Notes   string
	Tags    []string
	DueDate *string // canonical "YYYY-MM-DD"; nil when caller omitted the field
}

// taskQueue is the in-memory buffer behind ctx.queueTask. Each Enqueue
// validates synchronously so the script gets immediate feedback on bad
// input, even though persistence is deferred.
type taskQueue struct {
	items []queuedTask
}

// newTaskQueue returns an empty queue.
func newTaskQueue() *taskQueue {
	return &taskQueue{}
}

// Enqueue validates raw (a JSON-style map taken from JS) and appends a
// queuedTask. Returns an error on invalid input; the queue is unchanged on
// error.
func (q *taskQueue) Enqueue(raw map[string]any) error {
	title, _ := raw["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("runtime: queueTask: title is required")
	}

	notes, _ := raw["notes"].(string)

	var due *string
	if v, ok := raw["due_date"]; ok && v != nil {
		s, isStr := v.(string)
		if !isStr {
			return fmt.Errorf("runtime: queueTask: due_date must be a string")
		}
		s = strings.TrimSpace(s)
		if s != "" {
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return fmt.Errorf("runtime: queueTask: invalid due_date %q: %w", s, err)
			}
			due = &s
		}
	}

	tags, err := normalizeQueueTags(raw["tags"])
	if err != nil {
		return err
	}

	q.items = append(q.items, queuedTask{
		Title:   title,
		Notes:   notes,
		Tags:    tags,
		DueDate: due,
	})
	return nil
}

// Drain returns the buffered entries in enqueue order and resets the queue
// to empty. A second Drain on the same queue is well-defined and returns an
// empty slice.
func (q *taskQueue) Drain() []queuedTask {
	out := q.items
	q.items = nil
	if out == nil {
		out = []queuedTask{}
	}
	return out
}

// normalizeQueueTags accepts any of: nil, []any, or []string and returns a
// deduped, trimmed []string preserving first-appearance order. Any other
// shape is an error so user scripts can't accidentally pass a number where a
// tag name belongs.
func normalizeQueueTags(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	var items []any
	switch x := raw.(type) {
	case []any:
		items = x
	case []string:
		for _, s := range x {
			items = append(items, s)
		}
	default:
		return nil, fmt.Errorf("runtime: queueTask: tags must be an array of strings, got %T", raw)
	}

	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, fmt.Errorf("runtime: queueTask: tag must be a string, got %T", it)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

// queueBinding installs ctx.queueTask on rt. The JS-facing function takes a
// single object argument; validation errors propagate as JS Error objects so
// user scripts can catch them.
func queueBinding(rt *goja.Runtime, queue *taskQueue) func(input goja.Value) goja.Value {
	return func(input goja.Value) goja.Value {
		if input == nil || goja.IsUndefined(input) || goja.IsNull(input) {
			panic(rt.NewGoError(fmt.Errorf("runtime: queueTask: input is required")))
		}
		raw, ok := input.Export().(map[string]any)
		if !ok {
			panic(rt.NewGoError(fmt.Errorf("runtime: queueTask: input must be an object")))
		}
		if err := queue.Enqueue(raw); err != nil {
			panic(rt.NewGoError(err))
		}
		return goja.Undefined()
	}
}
