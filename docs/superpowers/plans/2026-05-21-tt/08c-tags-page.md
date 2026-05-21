# Phase 08c — `/tags` Page

> Read `00-index.md` first. Commit after each task. Parallelizable with 08a/b/d/e.

**Goal:** Managed tag list per spec §6: add, rename, delete (with cascade-confirm).

**Dependencies:** 06 (tag endpoints), 07.

**Tech stack:** shadcn `<Input>`, `<AlertDialog>`, react-hook-form.

**Parallelizable with:** 08a, 08b, 08d, 08e.

## File map

```
web/src/api/tags.ts            # extend with mutations
web/src/features/tags/
├── page.tsx
├── tag-row.tsx
├── add-tag-form.tsx
└── *.test.tsx
web/src/routes/tags.tsx        # replace placeholder
```

## Task 1: Mutations

**Files:** `web/src/api/tags.ts` (extend)

- [ ] Add:
  - `useCreateTag()` → `POST /tags`.
  - `useRenameTag()` → `PATCH /tags/:id`.
  - `useDeleteTag()` → `DELETE /tags/:id`. Invalidates `['tags']` and `['tasks']` (cascade affects task tags).
- [ ] Commit:
  ```bash
  git add web/src/api/tags.ts && git commit -m "feat(web): add tag mutation hooks"
  ```

## Task 2: Add-tag form

**Files:** `web/src/features/tags/add-tag-form.tsx`

- [ ] react-hook-form + zod single-field form. Submit on Enter or button click.
- [ ] On duplicate (HTTP 409 `code: conflict`), display "Tag already exists" inline.
- [ ] Commit:
  ```bash
  git add web/src/features/tags/add-tag-form.tsx && \
    git commit -m "feat(tags): add tag creation form"
  ```

## Task 3: Tag row with inline rename

**Files:** `web/src/features/tags/tag-row.tsx`

- [ ] Renders the tag name, an "Edit" pencil icon (clicking turns the name into an `<Input>`), and a "Delete" trash icon.
- [ ] Editing: Enter saves (calls `useRenameTag`), Esc cancels. Validation: non-empty.
- [ ] Delete: opens an `<AlertDialog>` with text "This will remove the tag from any tasks that use it. Continue?" (cascade-confirm per spec §6). On confirm, calls `useDeleteTag`.
- [ ] **Tests cover:**
  - Click "Edit", type a new name, press Enter → mutation invoked with `{id, name}`.
  - Click "Delete", confirm in dialog → `useDeleteTag` invoked.
- [ ] Commit:
  ```bash
  git add web/src/features/tags/tag-row.tsx && \
    git commit -m "feat(tags): add tag row with inline rename and cascade-delete"
  ```

## Task 4: Page assembly

**Files:** `web/src/features/tags/page.tsx`, `web/src/routes/tags.tsx`

- [ ] Layout: `<AddTagForm />` at the top, then a list of `<TagRow />` in alphabetical order.
- [ ] Empty state per spec: "No tags yet. Add one to start organizing tasks." (The add form is always present, so this is more of a "list area" placeholder.)
- [ ] Replace the placeholder route.
- [ ] Commit:
  ```bash
  git add web/src/features/tags/page.tsx web/src/routes/tags.tsx && \
    git commit -m "feat(tags): assemble /tags page"
  ```

## Phase completion checklist

- [ ] `pnpm run test` passes.
- [ ] Manual smoke: add a tag, rename it, delete it with confirm; verify a task that used the tag is still present but the tag is gone from its chip list.
