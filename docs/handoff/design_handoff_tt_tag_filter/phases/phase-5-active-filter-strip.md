# Phase 5 — Active filter strip joiners

## Goal

`ActiveFilterStrip` already lives above the table and renders the current
filter as removable chips. Update it for the new model:

1. Render each tag chip with an `or` / `and` joiner between them, matching
   `tag_filter.mode`.
2. Render the Untagged chip via the `untagged` `TagChip` variant from
   Phase 3.
3. Removing a tag chip mutates `tag_filter.tags` (not a top-level `tags`
   array).

## Files touched

- `web/src/features/tasks/active-filter-strip.tsx` — read `tag_filter`,
  render joiners, remove via `setSearch`

## Visual reference

See `reference/Tag Filter Refinement.html` §05 "Active-filter strip reads
naturally" for four example strip states (Any with two tags, All with two
tags, Untagged + one tag in Any, Untagged alone).

## Code patterns

### Read

```tsx
const { search, setSearch } = useTaskListSearch();
const filter = search.tag_filter;
const tags = filter?.tags ?? [];
const joiner = filter?.mode === 'all' ? 'and' : 'or';
```

### Remove

```tsx
function removeTag(name: string) {
  if (!filter) return;
  const next = filter.tags.filter((t) => t !== name);
  setSearch({
    tag_filter: next.length ? { ...filter, tags: next } : undefined,
  });
}
```

When the user removes the last non-Untagged tag while in All mode, the
result is `any:@untagged` if Untagged was selected, or no filter. Don't
preserve `mode: 'all'` with a single Untagged entry — `serializeTagFilter`
emits whichever mode it's given; flip to Any when the remaining set is just
`[UNTAGGED_TOKEN]`:

```tsx
function removeTag(name: string) {
  if (!filter) return;
  const next = filter.tags.filter((t) => t !== name);
  if (next.length === 0) {
    setSearch({ tag_filter: undefined });
    return;
  }
  // Drop All if it no longer makes sense (single tag, or only Untagged).
  const mode: TagMatchMode =
    next.length === 1 || next.every((t) => t === UNTAGGED_TOKEN) ? 'any' : filter.mode;
  setSearch({ tag_filter: { mode, tags: next } });
}
```

### Render

```tsx
<div className="flex flex-wrap items-center gap-1.5">
  <span className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
    Filter
  </span>

  {/* other filter pills: state, due, q — unchanged */}

  {tags.map((name, i) => (
    <Fragment key={name}>
      {i > 0 && (
        <span className="font-mono text-[10px] text-muted-foreground">{joiner}</span>
      )}
      <RemovableTagChip name={name} onRemove={() => removeTag(name)} />
    </Fragment>
  ))}

  {hasAnyFilter && (
    <button
      type="button"
      onClick={() => setSearch({ /* clear all filter keys */ })}
      className="ml-auto text-xs text-muted-foreground underline-offset-2 hover:underline"
    >
      Clear all
    </button>
  )}
</div>
```

`<RemovableTagChip>` is a thin wrapper around `<TagChip>` plus an `×` button.
It already exists if the strip currently renders removable chips — keep its
contract and pass the `untagged` variant through automatically (the chip
handles its own styling based on `name === UNTAGGED_TOKEN`).

## Acceptance

- Two real tags in Any mode → strip reads `work or errand` with `×` on each.
- Two real tags in All mode → strip reads `work and urgent`.
- Untagged + one real tag in Any → strip reads `Untagged or work` with the
  italic dashed Untagged chip.
- Untagged alone → strip reads `Untagged`.
- Removing the second-to-last tag in All mode auto-flips mode to Any.
- Removing the last tag clears the param.
- "Clear all" clears `tag_filter` (and other filter keys, as today).

## Tests

- Component test of `<ActiveFilterStrip>` for each of the four reference
  cases plus the All → Any flip on removal.
- Snapshot test for the four visual states.

## Dependencies

- Phase 1 (schema)
- Phase 3 (`TagChip` untagged variant)
- Independent of Phase 4 — can land in parallel.

## Pre-existing `tagsExclude` chips

The strip likely already renders excluded-tag chips (sourced from
`search.tagsExclude`) in their own group with their own removal handler.
**Leave that block untouched.** This phase only changes the include-side
chip rendering to add joiners between them. Read order in the strip stays:
state → due → q → include tags (with joiners) → exclude tags (unchanged).
