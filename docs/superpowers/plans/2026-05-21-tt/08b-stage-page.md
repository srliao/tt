# Phase 08b — `/stage` Page

> Read `00-index.md` first. Commit after each task. Parallelizable with 08a/c/d/e.

**Goal:** The focused stage list per spec §6. Reorderable via dnd-kit, soft cap > 7 hint, "Clear finished" + "Clear stage" buttons, done/cancelled rows visually de-emphasized but kept in place (their `staged_order` is preserved).

**Dependencies:** 06 (stage endpoints), 07.

**Tech stack:** dnd-kit, shadcn `<AlertDialog>` for clear-stage confirm.

**Parallelizable with:** 08a, 08c, 08d, 08e.

## File map

```
web/src/api/stage.ts
web/src/features/stage/
├── page.tsx
├── stage-list.tsx
├── stage-row.tsx
├── soft-cap-hint.tsx
└── *.test.tsx
web/src/routes/stage.tsx       # replace placeholder
```

## Task 1: API hooks

**Files:** `web/src/api/stage.ts`

- [ ] Implement:
  - `useStagedTasks()` → `useQuery(['tasks','staged'], () => api('/tasks?staged=true'))` — if a `?staged=true` filter doesn't exist, fall back to fetching `/tasks` and filtering client-side. **Better:** add a `?staged=true` URL param to `task.List` filter handling in phase 06. If discovered missing here, note in cross-phase findings and fall back to client-side filter via `tasks.filter(t => t.staged_order != null)`.
  - `useReorderStage()` → `POST /stage/reorder`.
  - `useClearStage()` → `DELETE /stage`.
  - `useClearFinishedFromStage()` → `DELETE /stage/finished`.
  - `useUnstageTask()` → reuse from 08a tasks hooks.
- [ ] Commit:
  ```bash
  git add web/src/api/stage.ts && git commit -m "feat(web): add stage query/mutation hooks"
  ```

## Task 2: Stage row component

**Files:** `web/src/features/stage/stage-row.tsx`

- [ ] Renders:
  - Drag handle (left).
  - Title (click → reuse the edit modal pattern from 08a, or a simpler inline edit; for v1 keep it as a click-through to the edit modal via a global event `tt:edit-task` carrying the id, listened to by a shared modal mounted at the root).
  - Due date chip (red if overdue).
  - Tag chips.
  - State toggle (cycles `not_done → done → cancelled → not_done`).
  - "Unstage" button (small icon).
- [ ] Done/cancelled rows: strikethrough title, desaturated background. The row remains in the visible list at its current position (no reordering on state change — verify by mutating state and re-rendering).
- [ ] **Tests cover:**
  - A `done` row gets the `line-through` + desaturated background classes.
  - Toggling state calls the correct mutation.
- [ ] Commit:
  ```bash
  git add web/src/features/stage/stage-row.tsx && \
    git commit -m "feat(stage): add row with state-aware styling"
  ```

## Task 3: Stage list with dnd-kit

**Files:** `web/src/features/stage/stage-list.tsx`

- [ ] Wrap rows in `<DndContext sensors=[pointer, keyboard]>` + `<SortableContext items=ids vertical>`.
- [ ] On drop:
  - Compute new `beforeID` / `afterID` (always the rows immediately above/below in the post-drop order — no filter complication since this list isn't filtered).
  - Call `useReorderStage().mutate({task_id, before_id, after_id})` with optimistic update.
- [ ] **Tests cover:** simulating a drop calls the mutation with the right neighbor ids.
- [ ] Commit:
  ```bash
  git add web/src/features/stage/stage-list.tsx && \
    git commit -m "feat(stage): add dnd-kit reorderable list"
  ```

## Task 4: Soft-cap hint

**Files:** `web/src/features/stage/soft-cap-hint.tsx`

- [ ] When the staged count > 7, render a dismissible inline hint above the list: `"Focused stages stay small — consider clearing finished items or unstaging anything that can wait."` Dismissal persists per session (`sessionStorage["tt.stage-cap-dismissed"]`).
- [ ] **Tests cover:** Hint absent at count = 7, visible at count = 8. Dismissing removes it for the session.
- [ ] Commit:
  ```bash
  git add web/src/features/stage/soft-cap-hint.tsx && \
    git commit -m "feat(stage): add soft-cap hint"
  ```

## Task 5: Page assembly with top bar + confirms

**Files:** `web/src/features/stage/page.tsx`, `web/src/routes/stage.tsx`

- [ ] Top bar:
  - "N staged" count.
  - Button "Clear finished" → calls `useClearFinishedFromStage`; no confirm (low-stakes).
  - Button "Clear stage" → opens shadcn `<AlertDialog>` confirm; on confirm calls `useClearStage`.
  - Button "Add from list →" — link to `/tasks`.
- [ ] Empty state per spec: "Nothing staged. Pick a few tasks from your list to focus on now." + button → `/tasks`.
- [ ] Replace `web/src/routes/stage.tsx` placeholder with `<StagePage />`.
- [ ] Per-page shortcuts (`j`/`k`/`enter`/`e`/`u`/`space`/`d`) wired similarly to 08a's table.
- [ ] **Tests cover:** the "Clear stage" button only triggers the mutation after the confirm is accepted.
- [ ] Commit:
  ```bash
  git add web/src/features/stage/page.tsx web/src/routes/stage.tsx && \
    git commit -m "feat(stage): assemble /stage page with top bar and confirms"
  ```

## Phase completion checklist

- [ ] `pnpm run test` passes.
- [ ] Manual smoke: stage list renders staged tasks, reorders via drag, "Clear finished" leaves not_done items, "Clear stage" prompts then clears all, soft cap hint shows past 7, done rows stay in place with strikethrough.
