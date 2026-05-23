# Phase 4 — Sidebar Variant A

## Goal

Land the main UX change. Replace the existing `<TagMultiSelect>` popover in
`filter-sidebar.tsx` with an inline tag list:

1. **Pinned Untagged row** at the top of the list — italic label, dashed
   swatch, dashed divider below.
2. **Any / All segmented control** on the right side of the Tags section
   heading. Defaults to Any.
3. **Live "Matches" summary** at the bottom of the section showing the
   selected chips joined with `or` / `and`, total task count, and a Clear
   link.
4. **All + Untagged guard**: selecting Untagged while All is active flips the
   mode to Any and clears non-Untagged tags. The All button is disabled when
   Untagged is selected, with a tooltip.

## Files touched

- `web/src/features/tasks/filter-sidebar.tsx` — replace `<TagMultiSelect>` and
  rewrite the Tags `<Section>`
- (No new file. The current `<TagMultiSelect>` function can be deleted in the
  same change.)

## Visual reference

See `reference/Tag Filter Refinement.html` §03 "Variant A — sidebar in-place
expansion" for the annotated mock (Before / After + numbered callouts).
Spacing/colors should match the existing app tokens, **not** the hex values
in the reference.

## Component shape

```tsx
function TagsSection() {
  const { search, setSearch } = useTaskListSearch();
  const { data: tags } = useTagsWithCounts();
  const filter = search.tag_filter;
  const selected = new Set(filter?.tags ?? []);
  const mode = filter?.mode ?? 'any';
  const untaggedSelected = selected.has(UNTAGGED_TOKEN);

  const update = (next: Set<string>, nextMode: TagMatchMode) => {
    const tags = Array.from(next);
    setSearch({
      tag_filter: tags.length ? { mode: nextMode, tags } : undefined,
    });
  };

  const toggle = (name: string) => {
    const next = new Set(selected);
    if (next.has(name)) {
      next.delete(name);
      update(next, mode);
      return;
    }
    next.add(name);

    // Guard: All + Untagged is impossible. When Untagged is added while All
    // is active, flip to Any and clear the non-Untagged entries.
    if (mode === 'all' && name === UNTAGGED_TOKEN) {
      update(new Set([UNTAGGED_TOKEN]), 'any');
      return;
    }
    update(next, mode);
  };

  const setMode = (next: TagMatchMode) => {
    // The All button is disabled when Untagged is selected, so this branch
    // shouldn't fire — but guard anyway.
    if (next === 'all' && untaggedSelected) return;
    update(selected, next);
  };

  // … render below
}
```

## Render

```tsx
<Section
  title="Tags"
  trailing={
    <SegmentedControl
      value={mode}
      onValueChange={setMode}
      items={[
        { value: 'any', label: 'Any' },
        {
          value: 'all',
          label: 'All',
          disabled: untaggedSelected,
          disabledReason: 'Untagged can only combine with Any.',
        },
      ]}
    />
  }
>
  {/* Pinned untagged row */}
  <TagPickerRow
    name={UNTAGGED_TOKEN}
    label={<span className="italic text-muted-foreground">Untagged</span>}
    swatch={<UntaggedSwatch />}
    count={untaggedCount}
    selected={untaggedSelected}
    onToggle={() => toggle(UNTAGGED_TOKEN)}
  />
  <div className="my-1 border-t border-dashed border-border" />

  {/* Real tags */}
  {(tags ?? []).map((t) => (
    <TagPickerRow
      key={t.id}
      name={t.name}
      label={t.name}
      swatch={<TagColorSwatch name={t.name} />}
      count={t.task_count}
      selected={selected.has(t.name)}
      onToggle={() => toggle(t.name)}
    />
  ))}

  {/* Live summary */}
  {selected.size > 0 && <MatchesSummary mode={mode} selected={selected} />}
</Section>
```

### `<TagPickerRow>`

A small internal component — checkbox + swatch + label + count, hover
background. The whole row is clickable (clicking the row toggles, same as
clicking the checkbox). No popover.

```tsx
function TagPickerRow({ selected, onToggle, swatch, label, count }: …) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        'flex w-full items-center gap-2 rounded px-1.5 py-1 text-sm',
        'hover:bg-accent',
      )}
    >
      <Checkbox checked={selected} tabIndex={-1} />
      {swatch}
      <span className="flex-1 text-left">{label}</span>
      <span className="font-mono text-xs text-muted-foreground">{count}</span>
    </button>
  );
}
```

### `<MatchesSummary>`

Renders the current selection as chips joined with the mode word, plus a
task count and Clear link. Uses `<TagChip>` (Phase 3) for both real tags and
the Untagged variant.

```tsx
function MatchesSummary({ mode, selected }: { mode: TagMatchMode; selected: Set<string> }) {
  const items = Array.from(selected);
  const joiner = mode === 'any' ? 'or' : 'and';
  return (
    <div className="mt-3 border-t border-dashed border-border pt-3">
      <div className="mb-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
        Matches
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        {items.map((name, i) => (
          <Fragment key={name}>
            {i > 0 && (
              <span className="font-mono text-[10px] text-muted-foreground">{joiner}</span>
            )}
            <TagChip name={name} />
          </Fragment>
        ))}
      </div>
      {/* Task count comes from the live useTasks() result in the page;
          pipe it in via a prop or read from a shared context. */}
    </div>
  );
}
```

`useTagsWithCounts()` already returns `task_count`; if it doesn't include the
untagged count, extend the existing `/tags?counts=1` endpoint to also report
the count of tasks with zero tags (single extra row, no schema change).
Otherwise, derive it client-side from the full task list — acceptable for a
single-user local app.

## Behavior details

- **Empty selection** → no `tag_filter` in URL; the section heading shows the
  Any/All seg control with Any active but it has no effect.
- **Clear link** in `<MatchesSummary>` calls `setSearch({ tag_filter: undefined })`.
- **Click anywhere on a row** toggles the tag (don't require hitting the
  checkbox).
- **Keyboard**: each row is a `<button>`; existing focus styles from shadcn
  apply. No special j/k handling — that's the table's affair.
- **Untagged + All disabled state** — the All button shows a tooltip
  `Untagged can only combine with Any.`. If the user selects Untagged while
  All is active (e.g. via the palette), auto-flip per the guard above.

## Acceptance

- The Tags section shows a pinned `Untagged` row at the top, with a dashed
  swatch and italic label, and a dashed divider below it.
- The Tags section heading has a small Any / All segmented control. Default
  Any; persists in `tag_filter.mode`.
- Clicking a row (anywhere) toggles its checkbox and updates `tag_filter`.
- Selecting Untagged while All is active flips the mode to Any and discards
  any other selected tags.
- When Untagged is selected, the All button is disabled and shows the
  tooltip on hover/focus.
- The "Matches" summary appears only when ≥1 tag is selected and renders
  `chip [or|and] chip [or|and] chip` correctly.
- The popover-based `TagMultiSelect` is removed and unused; no callers remain.

## Tests

- Component test for `<TagsSection>`:
  - selects a real tag → URL updates to `tag_filter=any:work`.
  - selects two real tags → `any:work,errand`; result list reflects union.
  - flips to All → `all:work,errand`; result list reflects intersection.
  - selects Untagged → `any:@untagged`.
  - selects Untagged while All is active → mode flips to Any, non-Untagged
    tags clear, URL = `any:@untagged`.
  - All button disabled when Untagged is selected.
- Visual regression for the section in three states: empty, two real tags
  selected, Untagged + one real tag selected.

## Dependencies

- Phase 1 (schema + setters)
- Phase 3 (`TagChip` untagged variant; `TaskListParams.tag_filter`)
- Optional: Phase 2 (so the result list actually reflects the new filter at
  runtime). The UI itself can land without the backend; the dev sees the URL
  change but the list won't filter correctly until Phase 2 ships. Land both
  before merging to main.
