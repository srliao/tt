package runtime

import (
	"context"
	"fmt"

	"github.com/dop251/goja"
	"github.com/srliao/tt/internal/script"
)

// logBindings wires up the immediate-flush logging surface for one run. The
// returned logFn synchronously calls AppendLog so logs survive an error or
// timeout that follows. The installLog helper attaches ctx.log (callable +
// debug/info/warn/error sub-methods) and the console.{log,info,warn,error}
// aliases that share the same backing fn.
//
// "Callable map" in goja terms is awkward to express purely from Go: a single
// JS identifier can be either a callable function OR a property bag of methods,
// not both. We work around it by installing a Go-backed "impl" hook and then
// evaluating a one-line JS snippet that creates the function-with-properties
// shape — see installLog.
func logBindings(svc script.Service, runID int64) func(ctx context.Context, level script.LogLevel, msg string) error {
	return func(ctx context.Context, level script.LogLevel, msg string) error {
		if err := svc.AppendLog(ctx, runID, level, msg); err != nil {
			return fmt.Errorf("runtime: append log: %w", err)
		}
		return nil
	}
}

// installLog wires ctx.log + console into rt, routing every call into logFn.
// logFn errors are swallowed: a script log call that races database closure
// (e.g. during teardown) must never abort the running script.
func installLog(rt *goja.Runtime, ctx context.Context, logFn func(context.Context, script.LogLevel, string) error) error {
	impl := func(level string, msg string) {
		var lvl script.LogLevel
		switch level {
		case "debug":
			lvl = script.LogDebug
		case "warn":
			lvl = script.LogWarn
		case "error":
			lvl = script.LogError
		default:
			lvl = script.LogInfo
		}
		_ = logFn(ctx, lvl, msg)
	}
	if err := rt.Set("__logImpl__", impl); err != nil {
		return fmt.Errorf("runtime: set __logImpl__: %w", err)
	}
	// Install ctx.log as a callable function with debug/info/warn/error
	// sub-methods, plus a console object that aliases the same backing fn.
	// console.log maps to "info" to match Node-style conventions. The IIFE
	// captures __logImpl__ into a local so the trailing `delete` can hide
	// the global from user scripts without breaking the bindings.
	_, err := rt.RunString(`
        (function () {
            var impl = __logImpl__;
            var stringify = function (m) {
                if (m === undefined) return "undefined";
                if (m === null) return "null";
                if (typeof m === "string") return m;
                try { return JSON.stringify(m); } catch (e) { return String(m); }
            };
            var log = function (m) { impl("info", stringify(m)); };
            log.debug = function (m) { impl("debug", stringify(m)); };
            log.info  = function (m) { impl("info",  stringify(m)); };
            log.warn  = function (m) { impl("warn",  stringify(m)); };
            log.error = function (m) { impl("error", stringify(m)); };
            ctx.log = log;
            this.console = {
                log:   function (m) { impl("info",  stringify(m)); },
                info:  function (m) { impl("info",  stringify(m)); },
                warn:  function (m) { impl("warn",  stringify(m)); },
                error: function (m) { impl("error", stringify(m)); }
            };
        })();
        delete this.__logImpl__;
    `)
	if err != nil {
		return fmt.Errorf("runtime: install log bindings: %w", err)
	}
	return nil
}
