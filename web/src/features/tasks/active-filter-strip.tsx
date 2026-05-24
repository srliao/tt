/**
 * <ActiveFilterStrip> — a single horizontal row of removable chips that
 * echoes the user's current filter set. Hidden when no filter is active and
 * no state-restriction is in play.
 *
 * Lives between the page header and the task table on `/tasks`. The strip is
 * intentionally URL-driven (via `useTaskListSearch`) rather than receiving
 * props so it can be dropped into any future surface that uses the same
 * search schema (e.g. /stage) without re-plumbing state.
 *
 * Chip semantics:
 *   - `q`           → "..." chip; clear → q: undefined
 *   - `due`         → "due: today" chip; clear → due: undefined
 *   - `tags`        → solid <TagChip> per name; clear → drop one from array
 *   - `tagsExclude` → dim <TagChip> per name; clear → drop one from array
 *   - state-restricted → "Open only · include done?" pill that, when clicked,
 *     widens `states` back to the full set so done + cancelled appear.
 *
 * "Clear all" wipes every filter axis AND the quick-preset, returning the
 * page to its empty/default view.
 */

import { XIcon } from 'lucide-react';
import { Fragment } from 'react';
import { TagChip } from '@/components/ui/tag-chip';
import {
  hasActiveFilters,
  isStateRestricted,
  type TagMatchMode,
  UNTAGGED_TOKEN,
  useTaskListSearch,
} from './use-task-list-search';

export function ActiveFilterStrip() {
  const { search, setSearch } = useTaskListSearch();
  if (!hasActiveFilters(search) && !isStateRestricted(search)) return null;

  const filter = search.tag_filter;
  const includeTags = filter?.tags ?? [];
  // Joiner mirrors the filter's match mode: All → "and", Any → "or". Rendered
  // between chips (i > 0) so the strip reads as a natural sentence, e.g.
  // `work and urgent` or `Untagged or errand`.
  const joiner = filter?.mode === 'all' ? 'and' : 'or';

  // Removing a chip from the include set shrinks `tag_filter.tags`. When the
  // remaining set degenerates (size 1, or only the Untagged sentinel), `all`
  // no longer carries semantic weight, so we flip to `any` to keep the URL
  // canonical and the rendered joiner sensible.
  const removeIncludeTag = (name: string) => {
    if (!filter) return;
    const next = filter.tags.filter((t) => t !== name);
    if (next.length === 0) {
      setSearch({ tag_filter: undefined });
      return;
    }
    const mode: TagMatchMode =
      next.length === 1 || next.every((t) => t === UNTAGGED_TOKEN) ? 'any' : filter.mode;
    setSearch({ tag_filter: { mode, tags: next } });
  };

  return (
    <div
      data-slot="active-filter-strip"
      className="mb-2 flex flex-wrap items-center gap-1.5 px-1 py-1.5 text-xs text-muted-foreground"
    >
      <span className="mr-1 font-medium">Filters:</span>

      {search.q && (
        <FilterChip label={`"${search.q}"`} onRemove={() => setSearch({ q: undefined })} />
      )}
      {search.due && (
        <FilterChip label={`due: ${search.due}`} onRemove={() => setSearch({ due: undefined })} />
      )}
      {includeTags.map((t, i) => (
        <Fragment key={`inc-${t}`}>
          {i > 0 && (
            <span
              data-slot="tag-filter-joiner"
              className="font-mono text-[10px] text-muted-foreground"
            >
              {joiner}
            </span>
          )}
          <TagChip name={t} onRemove={() => removeIncludeTag(t)} />
        </Fragment>
      ))}
      {(search.tagsExclude ?? []).map((t) => (
        <TagChip
          key={`exc-${t}`}
          name={t}
          dim
          onRemove={() =>
            setSearch({ tagsExclude: (search.tagsExclude ?? []).filter((x) => x !== t) })
          }
        />
      ))}
      {isStateRestricted(search) && (
        <button
          type="button"
          data-slot="state-restricted-chip"
          onClick={() => setSearch({ states: ['not_done', 'done', 'cancelled'] })}
          className="rounded-full border border-dashed px-2 py-0.5 text-foreground hover:bg-accent hover:text-accent-foreground"
        >
          Open only · include done?
        </button>
      )}

      <button
        type="button"
        onClick={() =>
          setSearch({
            q: undefined,
            due: undefined,
            tag_filter: undefined,
            tagsExclude: undefined,
            quick: undefined,
            states: undefined,
          })
        }
        className="ml-auto text-xs underline-offset-2 hover:underline"
      >
        Clear all
      </button>
    </div>
  );
}

function FilterChip({ label, onRemove }: { label: string; onRemove: () => void }) {
  return (
    <span
      data-slot="filter-chip"
      className="inline-flex items-center gap-1 rounded-full border bg-background px-2 py-0.5 text-foreground"
    >
      {label}
      <button
        type="button"
        onClick={onRemove}
        aria-label={`Remove filter ${label}`}
        className="inline-flex items-center justify-center rounded-full hover:bg-accent"
      >
        <XIcon className="size-3" />
      </button>
    </span>
  );
}
