# Phase 1 — URL schema for `tag_filter=`

## Goal

Replace the multi-valued `tag=` URL param with a single structured
`tag_filter=<mode>:<name>,<name>,…` param. Lock the contract before any UI
or server work touches it.

The frontend reads/writes `tag_filter`. `@untagged` is the reserved sentinel
for the pseudo-tag.

## Files touched

- `web/src/features/tasks/use-task-list-search.ts` — schema + (de)serialiser
- `web/src/routes/tasks.tsx` — route `validateSearch` uses the same schema

## Contract

Parsed shape (the value `useTaskListSearch().search.tag_filter` returns):

```ts
type TagMatchMode = 'any' | 'all';

interface TagFilter {
  mode: TagMatchMode;
  /** Tag names. May include the reserved sentinel '@untagged'. */
  tags: string[];
}
```

URL form (single string, URL-encoded once):

```
tag_filter=any:work,errand
tag_filter=all:work,urgent
tag_filter=any:@untagged
tag_filter=any:@untagged,work
```

When `tags` is empty, omit the param entirely — never write `tag_filter=any:`.

## Code patterns

### Schema

```ts
// web/src/features/tasks/use-task-list-search.ts

export const TAG_MATCH_MODES = ['any', 'all'] as const;
export type TagMatchMode = (typeof TAG_MATCH_MODES)[number];

/** The pseudo-tag sentinel. Real tags can't contain '@' (validated server-side). */
export const UNTAGGED_TOKEN = '@untagged';

export const tagFilterSchema = z.object({
  mode: z.enum(TAG_MATCH_MODES),
  tags: z.array(z.string().min(1)).min(1),
});

export type TagFilter = z.infer<typeof tagFilterSchema>;
```

### Add to `taskSearchSchema`

```ts
export const taskSearchSchema = z
  .object({
    states: z.array(z.enum(TASK_STATES)).optional(),
    // tags: z.array(z.string()).optional(),    ← REMOVE
    tag_filter: z.string().optional()            // ← raw URL string; parsed below
      .transform((s) => (s ? parseTagFilter(s) : undefined))
      .pipe(tagFilterSchema.optional()),
    due: z.enum(TASK_DUE_RANGES).optional(),
    q: z.string().optional(),
    sort: z.enum(TASK_SORTS).optional(),
    asc: z.boolean().optional(),
    quick: z.enum(QUICK_FILTERS).optional(),
  })
  .partial();
```

If your TanStack Router version routes the value as a string before
`validateSearch`, the `.transform()` above runs first. If it passes through
typed objects (e.g. JSON-encoded in the URL), use a custom serialiser instead
— see "Router integration" below.

### (De)serialiser

```ts
// web/src/features/tasks/use-task-list-search.ts

export function parseTagFilter(raw: string): TagFilter | undefined {
  const idx = raw.indexOf(':');
  if (idx < 0) return undefined;
  const mode = raw.slice(0, idx);
  if (mode !== 'any' && mode !== 'all') return undefined;
  const tags = raw
    .slice(idx + 1)
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
  if (tags.length === 0) return undefined;
  return { mode, tags };
}

export function serializeTagFilter(f: TagFilter | undefined): string | undefined {
  if (!f || f.tags.length === 0) return undefined;
  return `${f.mode}:${f.tags.join(',')}`;
}
```

The `setSearch` helper's existing cleanup loop drops empty/undefined values,
so passing `{ tag_filter: undefined }` clears the param. When writing a new
filter, pass the serialised string.

### Router integration

`useSearch` should expose the parsed object. Two viable shapes:

- **Option A (recommended).** The route's `validateSearch` runs the transform
  above and returns `tag_filter` as a `TagFilter | undefined`. The hook just
  reads `search.tag_filter`. `setSearch` accepts `TagFilter | undefined` and
  re-serialises in `navigate({ search: prev => ... })`.
- Option B. Keep the URL value as a raw string and parse on read with a
  memoised selector. Avoid — adds a parsing call at every consumer.

Go with Option A. The hook surface becomes:

```ts
const { search, setSearch } = useTaskListSearch();
// read:
search.tag_filter; // TagFilter | undefined
// write:
setSearch({ tag_filter: { mode: 'any', tags: ['work', '@untagged'] } });
// clear:
setSearch({ tag_filter: undefined });
```

`setSearch`'s navigate call must serialise the object back to a string for the
URL. Update the cleanup loop to call `serializeTagFilter` for that key.

### Quick filters

Quick filters in `applyQuickFilter()` don't set tags today. Leave them
unchanged — `tag_filter` flows through `base.tag_filter`. No quick filter
should preset a tag.

### `applyQuickFilter` shape

The function returns `TaskListParams` (the API shape). The API field name
changes in Phase 3. For now, pass `tag_filter` through unchanged:

```ts
const base: TaskListParams = {
  states: search.states && search.states.length > 0 ? search.states : undefined,
  // tags: search.tags && search.tags.length > 0 ? search.tags : undefined,   ← REMOVE
  tag_filter: search.tag_filter,
  due: search.due,
  q: search.q,
  sort: search.sort,
  asc: search.asc,
};
```

(The `TaskListParams` type change lives in Phase 3.)

## Acceptance

- `taskSearchSchema` no longer has a `tags` field; it has `tag_filter`.
- `parseTagFilter('any:work,errand')` returns `{ mode: 'any', tags: ['work', 'errand'] }`.
- `parseTagFilter('all:work,@untagged')` returns `{ mode: 'all', tags: ['work', '@untagged'] }`.
- `parseTagFilter('garbage')` returns `undefined`.
- `serializeTagFilter({ mode: 'any', tags: [] })` returns `undefined`.
- `setSearch({ tag_filter: undefined })` strips the param from the URL.
- Round-trip: `parseTagFilter(serializeTagFilter(x)!) === x` for any valid `TagFilter`.

## Tests

- Unit tests for `parseTagFilter` / `serializeTagFilter` covering: empty, single
  tag, multiple tags, untagged sentinel, mixed, malformed input.
- Route validation test: `/tasks?tag_filter=any:work,errand` resolves to a
  populated `tag_filter` on the parsed search; `/tasks` resolves to
  `tag_filter: undefined`.

## Dependencies

None — this is the first phase. Phases 2 – 6 all depend on this contract.

## Pre-existing code to retarget (not rewrite)

`use-task-list-search.ts` already exports **`useTagFilterMutator`** and a
**`clickModeFromEvent`** helper, used by row-chip clicks in
`task-table.tsx` and by the palette. The mutator currently reads/writes the
old `tags` + `tagMode` pair on the include side, and `tagsExclude` on the
exclude side.

In this phase, **retarget only the include path** to `tag_filter`:

- Include click → `setSearch({ tag_filter: { mode: prev?.mode ?? 'any', tags: [...prev?.tags ?? [], name] } })`
  (deduped; no-op if already present).
- Clear-from-include click → remove `name` from `tag_filter.tags`; if empty,
  set `tag_filter: undefined`. Same auto-flip-to-Any rule as
  `ActiveFilterStrip` (Phase 5) when the remainder is a single tag or just
  Untagged.
- **Exclude path is unchanged** — keeps writing `tagsExclude`.

Call sites (`task-table.tsx`'s `onTagClick`, `command-palette.tsx`) do **not**
change — the mutator's external signature stays `(name, clickMode)`.
