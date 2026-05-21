package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/srliao/tt/internal/script"
	"github.com/srliao/tt/internal/task"
)

// ctxDeps bundles everything installCtx needs from the surrounding Run. It's
// passed by value because each Run constructs a fresh one; sharing is never
// desired.
type ctxDeps struct {
	rt       *goja.Runtime
	now      time.Time
	sc       script.Script
	trigger  script.Trigger
	state    *stateBuffer
	queue    *taskQueue
	logFn    func(context.Context, script.LogLevel, string) error
	runCtx   context.Context
	lastTask *task.Task
}

// installCtx builds the ctx object exposed to every userscript. Order of
// installation matters only for ctx.log (which must exist before any other
// binding can call into it), so we install ctx itself first, then the log
// surface, then everything else.
//
// Sandbox enforcement happens here too: after the ctx tree is in place, we
// delete any dangerous globals the engine might have introduced. goja
// doesn't expose setTimeout / setInterval / fetch / process by default, but
// the deletes act as a defensive guard against future engine changes.
func installCtx(d ctxDeps) error {
	rt := d.rt

	ctxObj := rt.NewObject()
	if err := rt.Set("ctx", ctxObj); err != nil {
		return fmt.Errorf("runtime: set ctx: %w", err)
	}

	// Date helpers (ctx.today, weekday, …) — keep at the top because they
	// touch no other state.
	for name, fn := range dateBindings(rt, d.now) {
		if err := ctxObj.Set(name, fn); err != nil {
			return fmt.Errorf("runtime: set ctx.%s: %w", name, err)
		}
	}

	// ctx.log + console.* — install before anything else might want to log
	// at construction time.
	if err := installLog(rt, d.runCtx, d.logFn); err != nil {
		return err
	}

	// ctx.state.{get,set,delete,all}.
	if err := installState(rt, ctxObj, d.state); err != nil {
		return err
	}

	// ctx.queueTask.
	if err := ctxObj.Set("queueTask", queueBinding(rt, d.queue)); err != nil {
		return fmt.Errorf("runtime: set ctx.queueTask: %w", err)
	}

	// ctx.script metadata. lastRunAt is rendered as "YYYY-MM-DD HH:MM:SS"
	// UTC (the SQLite layout) so scripts can compare it as a string without
	// pulling in a parser.
	scriptMeta := map[string]any{
		"id":      d.sc.ID,
		"name":    d.sc.Name,
		"trigger": string(d.trigger),
	}
	if d.sc.LastRunAt != nil {
		scriptMeta["lastRunAt"] = d.sc.LastRunAt.UTC().Format("2006-01-02 15:04:05")
	} else {
		scriptMeta["lastRunAt"] = nil
	}
	if err := ctxObj.Set("script", scriptMeta); err != nil {
		return fmt.Errorf("runtime: set ctx.script: %w", err)
	}

	// ctx.lastSpawn — pre-computed JSON shape of the most recently spawned
	// task, or null if the script has never spawned anything.
	if d.lastTask != nil {
		if err := ctxObj.Set("lastSpawn", taskToJSObject(*d.lastTask)); err != nil {
			return fmt.Errorf("runtime: set ctx.lastSpawn: %w", err)
		}
	} else {
		if err := ctxObj.Set("lastSpawn", nil); err != nil {
			return fmt.Errorf("runtime: set ctx.lastSpawn: %w", err)
		}
	}

	// Defensive sandbox sweep. Errors here are non-fatal — goja's globals
	// vary by version and we'd rather a missing global than a missing
	// runtime.
	_, _ = rt.RunString(`
        delete this.setTimeout;
        delete this.setInterval;
        delete this.fetch;
        delete this.process;
        delete this.require;
    `)

	return nil
}

// taskToJSObject renders a Task as the plain JS object exposed via
// ctx.lastSpawn. Time fields use the SQLite "YYYY-MM-DD HH:MM:SS" UTC layout
// for consistency with ctx.script.lastRunAt; nullable fields emit JS null.
func taskToJSObject(t task.Task) map[string]any {
	const ts = "2006-01-02 15:04:05"
	obj := map[string]any{
		"id":           t.ID,
		"title":        t.Title,
		"notes":        t.Notes,
		"state":        string(t.State),
		"due_date":     nilString(t.DueDate),
		"created_at":   t.CreatedAt.UTC().Format(ts),
		"completed_at": nilTime(t.CompletedAt, ts),
		"cancelled_at": nilTime(t.CancelledAt, ts),
		"tags":         t.Tags,
	}
	return obj
}

// nilString returns the dereferenced string or nil — chosen so the goja-side
// JSON encoder emits `null` instead of an empty string for absent values.
func nilString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// nilTime mirrors nilString for *time.Time.
func nilTime(p *time.Time, layout string) any {
	if p == nil {
		return nil
	}
	return p.UTC().Format(layout)
}
