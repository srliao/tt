# Handoff: tt — Tag filter refinement (Untagged + Any/All)

## Overview

The tag filter today is a multi-select that does implicit OR. This package adds three things
behind one model:

1. **Untagged** — a pinned pseudo-tag at the top of the tag list, selectable like any tag.
2. **Match mode** — Any (existing OR) / All (new AND), exposed as a small segmented control
   on the Tags section heading.
3. **Mixed sets** — `Untagged + tag(s)` in Any mode returns the union (untagged ∪ tagged-with-any).

| Group | Phases | Goal |
|---|---|---|
| **Contract** | 1, 2 | `tag_filter=` URL param + server resolves Untagged and Any/All in SQL |
| **UI** | 3 – 6 | Sidebar Variant A, active-filter strip joiners, palette parity, cleanup |

## About the design files

The HTML in `reference/Tag Filter Refinement.html` is a **design reference** —
annotated mocks showing intent, model, surfaces, edge cases, and the new URL
shape. Don't copy its HTML/CSS directly. Recreate inside the existing React +
TanStack Router + shadcn/ui codebase using established patterns.

## Surface decision: Variant A only

The design doc shows two surfaces (sidebar in-place and a dedicated popover).
**This handoff ships Variant A — the sidebar in-place expansion.** Variant B
(popover) is parked for a later round if the tag list grows past ~20 entries.

## Fidelity

**Mid-fidelity.** The model, copy, URL shape, and edge-case behavior are final.
Spacing/colors should match the existing app tokens (`web/src/index.css`,
shadcn primitives), not the hex values in the reference HTML.

## Scope note — pre-existing features to preserve

The live repo already has two tag-filter features that aren't in the design
reference and **stay intact** through this handoff:

- **`tagsExclude` URL param + exclude chips.** Tag exclusion is already
  shipped as a separate URL param (`tagsExclude=`). This handoff does **not**
  fold exclude into `tag_filter`. The two params coexist:
  `?tag_filter=any:work,@untagged&tagsExclude=wip` means "(untagged or
  tagged work) and not tagged wip." Future work can subsume exclude into a
  richer `tag_filter` encoding (e.g. `any:work;not:wip`); out of scope here.
- **`useTagFilterMutator(name, clickMode)`** in `use-task-list-search.ts`.
  This is the central include/exclude/clear mutator that row-chip clicks and
  the palette call into. Phase 1 **retargets** it to read/write `tag_filter`
  instead of the old `tags` + `tagMode` pair, while leaving its exclude path
  (the one that writes `tagsExclude`) unchanged. Call sites (`task-table.tsx`,
  `command-palette.tsx`, anywhere else) don't change.

When in doubt: if a piece of code touches `tagsExclude`, leave it alone.

## Decisions locked in this handoff

- **Untagged placement** — single source of truth: pinned at the top of the
  Tags list with a dashed swatch and italic label. Not also a checkbox or a
  quick filter. One way to express it.
- **Match mode** — global, Any/All, segmented control on the Tags section
  heading. Default Any (preserves today's behavior).
- **Untagged + tag(s) in Any** — union: tasks with zero tags, **or** any of the
  selected tags.
- **Untagged + tag(s) in All** — impossible set. When the user selects
  Untagged while All is active, **auto-flip to Any and clear the non-Untagged
  selections** (no transient empty state). When All is active and the user has
  Untagged selected, the All button is disabled with a tooltip.
- **URL shape** — single structured param `tag_filter=<mode>:<name>,<name>,…`,
  with `@untagged` as the reserved sentinel. Tag names containing `@` are
  already rejected by validation, so no collision.
- **Back-compat** — server accepts legacy repeated `tag=` params for one
  release (treated as `any:`); frontend only writes `tag_filter=`. Remove the
  legacy reader after.

## How to use this package

Each phase is a single markdown file with goal, files touched, code patterns,
acceptance, and tests. Phases are **ordered**. Phase 1 locks the contract;
phase 2 makes the server honor it. Phases 3 → 6 are the UI rollout. Phase 6
is the cleanup pass (remove legacy `tag=` writer, run the doc-sync pass).

Per `CLAUDE.md` in the repo: any new public service/endpoint MUST trigger a
doc-sync subagent pass over `docs/agent/` and root `CLAUDE.md`. This handoff
**modifies** an existing endpoint (`GET /tasks`) rather than adding one, but
the URL shape changes — phase 6 includes the doc-sync. No new keyboard
shortcut is added, so `shortcut-cheatsheet.tsx` is untouched.

## Phases

| # | File | Ships | Touches |
|---|---|---|---|
| 1 | `phases/phase-1-url-schema.md` | `tag_filter=` parse/serialise; Zod schema; quick-filter migration | `use-task-list-search.ts`, route `validateSearch` |
| 2 | `phases/phase-2-backend.md` | Server parses `tag_filter`; SQL for Untagged + Any/All; legacy `tag=` reader | `internal/httpapi`, `internal/task`, `internal/db/queries` |
| 3 | `phases/phase-3-api-client-and-chip.md` | Frontend API client uses `tag_filter`; `TagChip` gains `untagged` variant | `api/tasks.ts`, `components/ui/tag-chip.tsx` |
| 4 | `phases/phase-4-sidebar-variant-a.md` | Pinned Untagged row + Any/All segmented control + All-vs-Untagged guard | `features/tasks/filter-sidebar.tsx` |
| 5 | `phases/phase-5-active-filter-strip.md` | `or`/`and` joiners between tag chips; Untagged chip variant | `features/tasks/active-filter-strip.tsx` |
| 6 | `phases/phase-6-palette-and-cleanup.md` | "Filter by: Untagged" palette entry; drop legacy `tag=` writer; doc sync | `components/command-palette.tsx`, `internal/httpapi`, `docs/agent/` |

## File-touch summary

```
web/src/
├── api/
│   └── tasks.ts                       (Phase 3)
├── components/
│   ├── command-palette.tsx            (Phase 6)
│   └── ui/
│       └── tag-chip.tsx               (Phase 3: untagged variant)
└── features/tasks/
    ├── filter-sidebar.tsx             (Phase 4)
    ├── active-filter-strip.tsx        (Phase 5)
    └── use-task-list-search.ts        (Phase 1)

internal/
├── db/queries/tasks.sql               (Phase 2: ListTasks rewrite for tag_filter)
├── httpapi/tasks_handler.go           (Phase 2; legacy reader removed in Phase 6)
└── task/service.go                    (Phase 2: TagFilter struct)

docs/agent/
└── ... (Phase 6: doc-sync subagent)
```

## Files

- `README.md` — this file
- `phases/phase-1-url-schema.md` … `phase-6-palette-and-cleanup.md` — phase-by-phase instructions
- `reference/Tag Filter Refinement.html` — the visual design reference (open in a browser)
