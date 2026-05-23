# Phase 6 — Command palette + cleanup + doc sync

## Goal

Wrap up: add Untagged to the command palette so the keyboard surface stays at
parity with the sidebar, drop the legacy `tag=` reader on the server, and run
the doc-sync subagent over the changed `GET /tasks` contract.

## Files touched

- `web/src/components/command-palette.tsx` — new "Filter by: Untagged" entry
- `internal/httpapi/tasks_handler.go` — remove legacy `tag=` reader
- `docs/agent/` + root `CLAUDE.md` — doc-sync pass

## Code patterns

### Palette entry

The palette already renders one `<CommandItem>` per real tag under a "Filter
by tag" group. Add a single entry **above** the per-tag entries:

```tsx
<CommandGroup heading="Filter by tag">
  <CommandItem
    value="untagged"
    onSelect={() => filterByUntagged()}
  >
    <TagChip name={UNTAGGED_TOKEN} />
    <span className="ml-auto text-xs text-muted-foreground">
      filter by tag
    </span>
  </CommandItem>
  {tags.map((t) => (
    <CommandItem … />
  ))}
</CommandGroup>
```

`filterByUntagged` navigates with the new param:

```tsx
const filterByUntagged = useCallback(() => {
  void navigate({
    to: '/tasks',
    search: (prev) => ({
      ...prev,
      tag_filter: { mode: 'any' as const, tags: [UNTAGGED_TOKEN] },
    }),
  });
  setOpen(false);
}, [navigate]);
```

The existing per-tag `filterByTag(name)` becomes:

```tsx
const filterByTag = useCallback(
  (name: string) => {
    void navigate({
      to: '/tasks',
      search: (prev) => ({
        ...prev,
        tag_filter: { mode: 'any' as const, tags: [name] },
      }),
    });
    setOpen(false);
  },
  [navigate],
);
```

Note: this **replaces** any existing tag filter — selecting a tag from the
palette is "show me only this tag", not "add to the current selection". This
matches today's behavior.

### Drop the legacy reader

```go
// internal/httpapi/tasks_handler.go (from Phase 2)

func parseTagFilter(q url.Values) task.TagFilter {
    if raw := q.Get("tag_filter"); raw != "" {
        // … unchanged …
    }
    // REMOVE: the `tag=` legacy fallback block.
    return task.TagFilter{}
}
```

Grep the test suite for any test that hits `?tag=` directly and migrate them
to `?tag_filter=any:…`. Add one regression test that confirms `?tag=foo` now
returns the unfiltered list (the param is silently ignored).

### Doc sync

Run the documented doc-sync flow per repo `CLAUDE.md`. Files that almost
certainly need updates:

- `docs/agent/tasks-api.md` (or equivalent) — replace the `tag=` param block
  with `tag_filter=`, document `@untagged` sentinel and Any/All semantics.
- `internal/httpapi/tasks.go` (top-of-file comment that documents the URL
  shape) — same replacement.
- Root `CLAUDE.md` — if it includes a one-line example of the tasks API
  query, update it.

No new shortcut is added, so `web/src/components/shortcut-cheatsheet.tsx`
stays untouched. Confirm this in the doc-sync pass.

## Acceptance

- Opening the palette and typing "untag" surfaces a single "Untagged" entry
  at the top of the tag group. Selecting it navigates to
  `/tasks?tag_filter=any:%40untagged` and shows untagged tasks.
- Selecting a real tag from the palette navigates to
  `/tasks?tag_filter=any:<name>`.
- `GET /tasks?tag=work` returns the unfiltered list (legacy reader gone).
- `docs/agent/` and the root `CLAUDE.md` reflect the new `tag_filter` param
  and the `@untagged` sentinel. No stale `tag=` examples remain.
- Cheatsheet (`shortcut-cheatsheet.tsx`) is unchanged.

## Tests

- Palette E2E: open palette → type "untag" → press Enter → URL becomes
  `tag_filter=any:%40untagged` and the list shows untagged tasks only.
- HTTP regression: `?tag=foo` returns the same body as `?` (unfiltered).
- Grep the repo for `tag=` URL examples in markdown — should be zero after
  the doc-sync pass.

## Dependencies

- Phases 1, 2, 3, 4, 5 must all be merged. Phase 6 is the closeout.
