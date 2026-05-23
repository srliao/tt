# Phase 3 — Frontend API client + `TagChip` untagged variant

## Goal

Two small foundations that the sidebar (Phase 4) and active-filter strip
(Phase 5) both depend on:

1. The API client encodes `tag_filter=` from the new `TagFilter` shape.
2. `<TagChip>` gains an `untagged` variant — italic label, dashed swatch,
   muted color, no hash-derived color.

## Files touched

- `web/src/api/tasks.ts` — `TaskListParams.tags` becomes `tag_filter?: TagFilter`
- `web/src/components/ui/tag-chip.tsx` — add `untagged` variant

## Code patterns

### API client

```ts
// web/src/api/tasks.ts

import {
  serializeTagFilter,
  type TagFilter,
} from '@/features/tasks/use-task-list-search';

export interface TaskListParams {
  states?: TaskState[];
  // tags?: string[];                  ← REMOVE
  tag_filter?: TagFilter;              // ← ADD
  due?: TaskDueRange;
  q?: string;
  sort?: TaskSortAxis;
  asc?: boolean;
  limit?: number;
  offset?: number;
}

export function buildTaskListQuery(params: TaskListParams): string {
  const sp = new URLSearchParams();
  for (const s of params.states ?? []) sp.append('state', s);
  const tf = serializeTagFilter(params.tag_filter);
  if (tf) sp.set('tag_filter', tf);   // ← replaces the `for (t of tags)` loop
  if (params.due) sp.set('due', params.due);
  if (params.q) sp.set('q', params.q);
  if (params.sort) sp.set('sort', params.sort);
  if (params.asc !== undefined) sp.set('asc', String(params.asc));
  if (params.limit !== undefined) sp.set('limit', String(params.limit));
  if (params.offset !== undefined) sp.set('offset', String(params.offset));
  const qs = sp.toString();
  return qs ? `?${qs}` : '';
}
```

`useTasks(params)` already includes the whole `params` object in its query
key, so swapping the field name automatically invalidates cached entries.

### TagChip variant

```tsx
// web/src/components/ui/tag-chip.tsx

import { UNTAGGED_TOKEN } from '@/features/tasks/use-task-list-search';

interface TagChipProps {
  name: string;                       // pass UNTAGGED_TOKEN for the pseudo-tag
  size?: 'sm' | 'md';
  className?: string;
}

export function TagChip({ name, size = 'sm', className }: TagChipProps) {
  if (name === UNTAGGED_TOKEN) {
    return (
      <span
        className={cn(
          'inline-flex items-center gap-1.5 rounded',
          'border border-dashed border-muted-foreground/40',
          'px-2 py-0.5 text-xs italic text-muted-foreground',
          size === 'md' && 'text-sm',
          className,
        )}
      >
        <span
          aria-hidden
          className="inline-block size-1.5 rounded-full border border-dashed border-muted-foreground/60"
        />
        Untagged
      </span>
    );
  }

  // existing color-hashed path unchanged …
}
```

If the existing `TagChip` exposes `onRemove` or click handlers, the untagged
variant should accept them on the same props — no behavioral special-casing.

### Display name

Wherever the Untagged chip is rendered in body copy (active-filter strip,
palette command, sidebar pinned row) the visible label is `Untagged` —
**not** `@untagged`. The sentinel is a wire token, not user-facing copy.

## Acceptance

- `buildTaskListQuery({ tag_filter: { mode: 'any', tags: ['work', '@untagged'] } })`
  returns `?tag_filter=any%3Awork%2C%40untagged` (or the equivalent un-encoded
  form — `URLSearchParams` handles encoding).
- `buildTaskListQuery({})` returns `''`.
- `buildTaskListQuery({ tag_filter: { mode: 'any', tags: [] } })` returns `''`
  (empty `tags` ⇒ no param).
- `<TagChip name="@untagged" />` renders the italic Untagged variant with a
  dashed swatch. `<TagChip name="work" />` renders the existing color-hashed
  variant.
- All existing `TagChip` callers compile without changes.

## Tests

- Unit test for `buildTaskListQuery` covering the new path.
- Visual / snapshot test for `<TagChip>` in both variants.
- Type test: `TaskListParams` no longer accepts `tags: string[]`.

## Dependencies

- Phase 1 — imports `serializeTagFilter`, `UNTAGGED_TOKEN`, `TagFilter`.
