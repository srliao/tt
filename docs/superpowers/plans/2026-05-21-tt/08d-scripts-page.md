# Phase 08d — `/scripts` List + Editor

> Read `00-index.md` first. Commit after each task. Parallelizable with 08a/b/c/e.

**Goal:** Scripts list page (`/scripts`) and the script editor (`/scripts/$id` and `/scripts/new`). Editor includes name field, enabled toggle, schedule sub-form with the `every_tick` confirmation, CodeMirror 6 editor, "Run now" button, recent-runs table, cheatsheet sidebar (API + tag list, both click-to-copy), and a "Spawned tasks" panel backed by `GET /scripts/:id/tasks`.

**Dependencies:** 06 (script endpoints), 07.

**Tech stack:** `@uiw/react-codemirror`, `@codemirror/lang-javascript`, react-hook-form + zod, shadcn `<Select>`, `<Switch>`, `<Tabs>`, lucide-react.

**Parallelizable with:** 08a, 08b, 08c, 08e.

## File map

```
web/src/api/scripts.ts                # query/mutation hooks for scripts + runs + spawned tasks
web/src/features/scripts/
├── list-page.tsx
├── editor-page.tsx
├── schedule-sub-form.tsx
├── code-editor.tsx
├── cheatsheet-api.tsx
├── cheatsheet-tags.tsx
├── recent-runs-table.tsx
├── spawned-tasks-panel.tsx
└── *.test.tsx
web/src/assets/ctx-cheatsheet.md     # static markdown of ctx API one-liners
web/src/routes/scripts.index.tsx     # replace placeholder
web/src/routes/scripts.$id.tsx
web/src/routes/scripts.new.tsx
```

## Task 1: API hooks

**Files:** `web/src/api/scripts.ts`

- [ ] Implement TanStack Query hooks:
  - `useScripts()`, `useScript(id)`.
  - `useCreateScript()`, `useUpdateScript()`, `useDeleteScript()`.
  - `useRunScript()` → `POST /scripts/:id/run`. Returns `{run_id}`. Mutation onSuccess: invalidate `['scripts', id, 'runs']` and navigate to `/runs/$id`.
  - `useScriptRuns(id, {limit, before})`.
  - `useSpawnedTasks(id, {cursor, limit})`.
- [ ] Commit:
  ```bash
  git add web/src/api/scripts.ts && git commit -m "feat(web): add script query/mutation hooks"
  ```

## Task 2: Cheatsheet asset

**Files:** `web/src/assets/ctx-cheatsheet.md`

- [ ] Write a static markdown file listing every `ctx.*` function from spec §5 with one-line descriptions. This is bundled into the SPA build and rendered as the cheatsheet sidebar.
- [ ] Use Vite's `?raw` import in `cheatsheet-api.tsx` to load it as a string.
- [ ] Commit:
  ```bash
  git add web/src/assets/ctx-cheatsheet.md && \
    git commit -m "docs(scripts): add ctx API cheatsheet markdown"
  ```

## Task 3: Scripts list page

**Files:** `web/src/features/scripts/list-page.tsx`, `web/src/routes/scripts.index.tsx`

- [ ] Table with columns: name (link → `/scripts/$id`), schedule (humanized: "Every 15 min", "Daily", "Weekly on Mon", "Monthly day 15", "Monthly last day"), enabled toggle, last run status pill + time, kebab menu (Run now, Delete).
- [ ] "New script" button (top-right) → navigates to `/scripts/new`.
- [ ] Empty state per spec §6: "Userscripts let you auto-create tasks on a schedule. Examples: weekly review, monthly bills, after-N-days follow-ups." + "Create your first script" button. Optionally pre-seed one disabled example script (skip for v1 — leave a TODO comment).
- [ ] Commit:
  ```bash
  git add web/src/features/scripts/list-page.tsx web/src/routes/scripts.index.tsx && \
    git commit -m "feat(scripts): add list page with empty state"
  ```

## Task 4: Schedule sub-form

**Files:** `web/src/features/scripts/schedule-sub-form.tsx`

- [ ] Controlled by the parent's react-hook-form. Renders:
  - A `<Select>` with options `every_tick | daily | weekly | monthly`.
  - When `weekly`: a weekday `<Select>` (Mon..Sun).
  - When `monthly`: a `<Select>` "Day of month" with `1..31` plus "Last".
  - When `every_tick`: render a yellow inline banner per spec: "Every-tick scripts run on every global tick (currently every 15 min). Buggy scripts can flood your task list. Confirm to use this schedule." + a checkbox "I understand". Form submission blocked until checked.
- [ ] Zod schema validates the kind + conditional fields.
- [ ] **Tests cover:**
  - Picking `every_tick` and trying to submit without the confirm box shows a validation error.
  - Switching from `weekly` to `monthly` clears the weekday and asks for the day.
- [ ] Commit:
  ```bash
  git add web/src/features/scripts/schedule-sub-form.tsx && \
    git commit -m "feat(scripts): add schedule sub-form with every_tick confirmation"
  ```

## Task 5: Code editor

**Files:** `web/src/features/scripts/code-editor.tsx`

- [ ] Install:
  ```bash
  cd web && pnpm add @uiw/react-codemirror @codemirror/lang-javascript
  ```
- [ ] Wrap `CodeMirror` with `extensions={[javascript()]}`. Height: `min-h: 50vh`.
- [ ] Light/dark theme: respond to the app theme (read from the ThemeProvider context).
- [ ] No lint plugin in v1.
- [ ] Commit:
  ```bash
  git add web/src/features/scripts/code-editor.tsx web/package.json && \
    git commit -m "feat(scripts): add CodeMirror editor"
  ```

## Task 6: Cheatsheet sidebar (API + Tags)

**Files:** `web/src/features/scripts/cheatsheet-api.tsx`, `web/src/features/scripts/cheatsheet-tags.tsx`

- [ ] `cheatsheet-api.tsx`:
  - Import the markdown asset via `import md from '@/assets/ctx-cheatsheet.md?raw'`.
  - Render with a tiny markdown→HTML transform (use `marked` or a custom split-by-h2 approach). Keep it lightweight; no syntax highlighting needed beyond inline `<code>`.
  - Each function name has a copy-to-clipboard button.
- [ ] `cheatsheet-tags.tsx`:
  - `useTags()` to list all tag names.
  - Each row: name + a copy-icon button that writes `"tagName"` (with surrounding quotes) to the clipboard, so scripts can paste it directly into a `tags: [...]` array.
- [ ] Wrap both in a collapsible side panel within the editor page.
- [ ] **Tests cover:** clicking the copy button on a tag writes the expected string to a mock clipboard.
- [ ] Commit:
  ```bash
  git add web/src/features/scripts/cheatsheet-* && \
    git commit -m "feat(scripts): add API and tag cheatsheet sidebar"
  ```

## Task 7: Recent runs table

**Files:** `web/src/features/scripts/recent-runs-table.tsx`

- [ ] Lists the last 20 runs (`useScriptRuns(id, {limit: 20})`): status pill, started_at (relative + absolute), duration (finished_at − started_at), spawned task count, link to `/runs/$id`.
- [ ] Auto-refetch every 5 seconds while the page is open (TanStack Query `refetchInterval`).
- [ ] Commit:
  ```bash
  git add web/src/features/scripts/recent-runs-table.tsx && \
    git commit -m "feat(scripts): add recent runs table"
  ```

## Task 8: Spawned tasks panel

**Files:** `web/src/features/scripts/spawned-tasks-panel.tsx`

- [ ] Paginated list via `useSpawnedTasks(id)`. Columns: title (link → edit modal), state pill, created_at (relative).
- [ ] "Load more" button if `nextCursor` returned.
- [ ] Commit:
  ```bash
  git add web/src/features/scripts/spawned-tasks-panel.tsx && \
    git commit -m "feat(scripts): add spawned tasks panel"
  ```

## Task 9: Editor page assembly

**Files:** `web/src/features/scripts/editor-page.tsx`, `web/src/routes/scripts.$id.tsx`, `web/src/routes/scripts.new.tsx`

- [ ] Layout (two-column when wide):
  - Left/main: header (editable name + enabled `<Switch>` + "Run now" button + delete kebab), schedule sub-form, code editor.
  - Right sidebar (`<Tabs>`): "API", "Tags", "Spawned tasks", "Recent runs".
- [ ] Form managed by react-hook-form. Submit on Cmd/Ctrl-Enter.
- [ ] Unsaved-changes guard: use `router.subscribe('onBeforeLoad', …)` or a navigation blocker to prompt "You have unsaved changes — leave anyway?" when `form.formState.isDirty`.
- [ ] "Run now" disabled when the form is dirty (must save first) and when `enabled === false`. On click: `useRunScript(id).mutate()` → on success, `navigate({to: '/runs/$id'})` with the returned `run_id`.
- [ ] `/scripts/new` route: same component with id-less mode. Submit calls `useCreateScript`, navigates to `/scripts/$id` on success.
- [ ] Replace placeholder routes.
- [ ] **Tests cover:** submitting the new-script form sends the right payload; submitting with `every_tick` but unchecked confirm shows validation; "Run now" disabled when form dirty.
- [ ] Commit:
  ```bash
  git add web/src/features/scripts/editor-page.tsx web/src/routes/scripts.*.tsx && \
    git commit -m "feat(scripts): assemble editor page with sidebar tabs and run-now"
  ```

## Phase completion checklist

- [ ] `pnpm run test` passes.
- [ ] Manual smoke:
  - Create a script, choose `every_tick`, see the warning, confirm, save.
  - Edit code, save, click "Run now" → navigates to `/runs/$id` and shows the run.
  - Confirm a task is created with `spawned_by_script_id` set.
  - Cheatsheet "API" and "Tags" panels copy to clipboard correctly.
  - Spawned tasks panel paginates.
