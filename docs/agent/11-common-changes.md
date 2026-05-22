# Common Change Recipes

Step-by-step trails for typical modifications. Use these as scaffolding — adapt to context.

## Add a field to `tasks`

1. **Migration**: `internal/db/migrations/000N_<name>.sql` with `ALTER TABLE tasks ADD COLUMN ...`. Add `-- +goose Down`.
2. **sqlc queries**: edit `internal/db/queries/tasks.sql` — RETURNING clauses already use `*` so SELECTs come back automatically; UPDATEs that touch the field need `SET ... = ?`.
3. **Regen**: `just db-gen`.
4. **Domain type**: add to `internal/task/types.go` (`Task` struct + relevant `*Input`).
5. **Projector**: update `rowToTask` in `internal/task/service.go`.
6. **Service methods**: if the field is mutable, update `Update` (or add a setter). Use input normalization (e.g., trimming, validation) before the SQL call.
7. **HTTP types**: update `taskBody` in `internal/httpapi/tasks.go` and the create/update handlers.
8. **Frontend types**: mirror in `web/src/types/task.ts`.
9. **Frontend forms/displays**: `web/src/features/tasks/edit-task-modal.tsx` (full edit surface), `web/src/features/tasks/add-task-modal.tsx` (title-only create modal triggered by `n` / "+ New task"), `task-row.tsx`, etc.
10. **Tests**: extend `internal/task/service_test.go`, `internal/httpapi/tasks_test.go`, frontend `task-table.test.tsx`.

## Add a new ctx method to userscripts

1. **Pick or create file** in `internal/runtime/` (`ctx_dates.go`, `ctx_state.go`, `ctx_queue.go`, or new).
2. **Implement the Go binding** as a closure or method on a struct. Use `rt.NewGoError(err)` to raise JS-visible errors.
3. **Wire into `installCtx`** in `internal/runtime/ctx.go` (or via a sub-installer like `installState`).
4. **Decide deferred vs immediate**:
   - Mutating? Defer through a buffer (like `taskQueue`) and flush only on `RunStatusOK`.
   - Read-only? Immediate is fine.
5. **Update cheatsheet** in `web/src/features/scripts/cheatsheet-api.tsx` so the editor sidebar shows the new method.
6. **Test** in `internal/runtime/runner_test.go`: write a JS snippet, run it, assert DB state / log rows / queued tasks.

## Add a new HTTP endpoint

See [07-http-api.md](./07-http-api.md) for the step-by-step. TL;DR:

1. Add the service method on the domain interface + impl.
2. Expose it on the narrow consumer interface in `internal/httpapi/server.go`.
3. Add the handler + route in the right `internal/httpapi/<file>.go`.
4. Add the API hook in `web/src/api/<resource>.ts`.
5. Test handler + hook.

## Add a quick filter to /tasks

1. Add the slug to `QUICK_FILTERS` in `web/src/features/tasks/use-task-list-search.ts`.
2. Add the case in `applyQuickFilter` translating slug → `TaskListParams`.
3. Add the menu item in the filter sidebar (`web/src/features/tasks/filter-sidebar.tsx`).
4. Update tests for `applyQuickFilter`.

## Add a new schedule kind

1. **Schema**: extend the `CHECK (schedule_kind IN (...))` constraint via migration.
2. **Domain**: add the constant in `internal/script/types.go` (`Kind` enum). If config has fields, extend `Schedule` struct + `MonthlyDay`-style tagged-union if needed.
3. **Parse**: update `ParseSchedule` + `Schedule.MarshalConfig` in `internal/script/schedule.go`.
4. **Match**: add the case in `Schedule.Matches`.
5. **HTTP wire format**: update `parseSchedule` in `internal/httpapi/scripts.go`.
6. **Frontend**: extend `scheduleSchema` (zod) in `web/src/features/scripts/editor-page.tsx` and the schedule sub-form `schedule-sub-form.tsx`.
7. **Type mirror**: `web/src/types/script.ts`.
8. **Tests**: extend `internal/script/schedule_test.go` truth table.

## Change the run timeout

`runtime.defaultTimeout` in `internal/runtime/runner.go`. Tests override via `runtime.WithTimeout(d)`. Spec mandates 5s; **change with care** — short timeouts make scripts hang on real workloads, long timeouts let a single bad script tax every tick.

## Change run retention

`runtime.runRetention` in `internal/runtime/runner.go` (currently 500). Pruning runs on every `Runner.Run` invocation. If you raise this significantly, consider whether the in-memory filter in `httpapi/runs.go:handleListRuns` (which fetches up to 500 rows then filters) needs to grow too.

## Add a column to a feature table (frontend)

1. Edit the page's table component (e.g., `web/src/features/tasks/task-table.tsx`):
   - Add the `<th>` to the thead row.
   - Pass any new prop to `TaskRow` / `SortableRow`.
2. Edit `task-row.tsx`: render the new `<td>`. Note the canonical row column order (checkbox / drag / done-toggle / title / bookmark) documented in [08-frontend.md](./08-frontend.md#task--stage-row-layout) — insert new columns inside the title block when possible to keep the tasks and stage rows symmetrical.
3. If the value is server-derived, ensure the `Task` type already includes it. Otherwise see "Add a field" above.
4. Update tests in `task-table.test.tsx`.

## Add a per-task destructive / state action

Row UI has no kebab menu — destructive / one-off state changes belong in the `EditTaskModal` footer (`web/src/features/tasks/edit-task-modal.tsx`). Pattern: add a footer `<Button>`; if it's destructive, gate behind an `AlertDialog` confirm (see the existing Delete button). The done/not-done toggle is the only state action that lives on the row itself.

## Add a global keyboard shortcut

Edit `web/src/lib/shortcuts.ts`:

1. Handle the key in the main `handler`. Use `event.preventDefault()` before the action.
2. For navigation, use `router.navigate({ to: '/...' })`.
3. For event-based actions (open modal, toggle panel), dispatch `new CustomEvent('tt:<action>')` and listen on the relevant page.
4. Suppress when `isEditableTarget(event.target)` returns true.
5. Update the cheatsheet (`web/src/components/shortcut-cheatsheet.tsx`).

## Debug a stuck "running" run

1. Visit `/runs` in the UI — the row with status=running and old started_at is the culprit.
2. Restart the binary — `RecoverOrphanedRuns` on startup will flip it to error with message `interrupted (binary restart)`.
3. Check stderr for the panic log. Stack trace will name the binding that crashed.
4. If the issue is a userscript infinite loop, the 5s `Interrupt` should have caught it — if not, check the `defaultTimeout` value and look for places where guard goroutine setup may have been skipped (rare).
