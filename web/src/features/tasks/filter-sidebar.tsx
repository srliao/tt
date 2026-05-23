/**
 * Left sidebar on the /tasks page: quick filters, state checkboxes, tag
 * picker, due-range select.
 *
 * All UI state lives in the URL via `useTaskListSearch()` so refreshes and
 * shared links stay stable. The in-sidebar search field has been moved to
 * the global command palette (`/` or `⌘K`); the `?q=` URL contract is
 * unchanged so deep links keep working.
 *
 * Phase 4 (Variant A): the tag section now lists every tag inline with a
 * pinned "Untagged" row at the top, an Any/All segmented control in the
 * heading, and a live "Matches" summary at the bottom. The popover-based
 * picker is gone; clicking anywhere on a row toggles it. Untagged + All is
 * disallowed: selecting Untagged while All is active flips mode to Any and
 * clears other selections, and the All button is disabled (with a tooltip)
 * whenever Untagged is selected.
 */

import { Fragment, type ReactNode, useMemo, useState } from 'react';
import { useTagsWithCounts } from '@/api/tags';
import { useTasks } from '@/api/tasks';
import { useTheme } from '@/components/theme-provider';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { TagChip } from '@/components/ui/tag-chip';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { tagColor, tagColorDark } from '@/lib/tag-color';
import { cn } from '@/lib/utils';
import type { TaskDueRange, TaskState } from '@/types/task';
import {
  isStateRestricted,
  QUICK_FILTERS,
  type QuickFilter,
  type TagMatchMode,
  UNTAGGED_TOKEN,
  useTaskListSearch,
} from './use-task-list-search';

const QUICK_LABELS: Record<QuickFilter, string> = {
  'all-open': 'All open',
  overdue: 'Overdue',
  'due-today': 'Due today',
  'recently-completed': 'Recently completed',
  cancelled: 'Cancelled',
};

const STATE_LABELS: Record<TaskState, string> = {
  not_done: 'Not done',
  done: 'Done',
  cancelled: 'Cancelled',
};

const ANY_SENTINEL = '__any__';

const DUE_OPTIONS: Array<{ value: string; range: TaskDueRange; label: string }> = [
  { value: ANY_SENTINEL, range: '', label: 'Any' },
  { value: 'overdue', range: 'overdue', label: 'Overdue' },
  { value: 'today', range: 'today', label: 'Today' },
  { value: 'this_week', range: 'this_week', label: 'This week' },
  { value: 'none', range: 'none', label: 'No due date' },
];

const UNTAGGED_TOOLTIP = 'Untagged can only combine with Any.';

export function FilterSidebar() {
  const { search, setSearch } = useTaskListSearch();

  return (
    <aside className="flex w-60 shrink-0 flex-col gap-4 p-4 text-sm">
      {isStateRestricted(search) && (
        <p className="text-xs text-muted-foreground">Searching open tasks only.</p>
      )}

      <Section title="Quick filters">
        <div className="flex flex-col gap-1">
          {QUICK_FILTERS.map((q) => (
            <button
              type="button"
              key={q}
              data-quick={q}
              data-active={search.quick === q || undefined}
              className="rounded-md px-2 py-1 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground data-[active]:bg-accent data-[active]:text-accent-foreground"
              onClick={() => setSearch({ quick: search.quick === q ? undefined : q })}
            >
              {QUICK_LABELS[q]}
            </button>
          ))}
        </div>
      </Section>

      <Separator />

      <Section title="State">
        <StateCheckboxes
          value={search.states ?? ['not_done']}
          onChange={(states) => setSearch({ states: states.length ? states : undefined })}
        />
      </Section>

      <Separator />

      <TagsSection />

      <Separator />

      <Section title="Due">
        <Select
          value={search.due ? search.due : ANY_SENTINEL}
          onValueChange={(v) =>
            setSearch({ due: v === ANY_SENTINEL ? undefined : (v as TaskDueRange) })
          }
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Any" />
          </SelectTrigger>
          <SelectContent>
            {DUE_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Section>
    </aside>
  );
}

function Section({ title, children }: { title: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</h2>
      {children}
    </div>
  );
}

function StateCheckboxes({
  value,
  onChange,
}: {
  value: TaskState[];
  onChange: (next: TaskState[]) => void;
}) {
  const toggle = (s: TaskState, checked: boolean) => {
    const set = new Set(value);
    if (checked) set.add(s);
    else set.delete(s);
    onChange(Array.from(set));
  };
  return (
    <div className="flex flex-col gap-1.5">
      {(Object.keys(STATE_LABELS) as TaskState[]).map((s) => {
        const id = `state-checkbox-${s}`;
        return (
          <div key={s} className="flex items-center gap-2 text-sm">
            <Checkbox
              id={id}
              checked={value.includes(s)}
              onCheckedChange={(checked) => toggle(s, checked === true)}
              data-testid={id}
            />
            <label htmlFor={id}>{STATE_LABELS[s]}</label>
          </div>
        );
      })}
    </div>
  );
}

/**
 * The Tags section — pinned "Untagged" row, dashed divider, then the full
 * tag inventory. The header carries an Any/All segmented control and the
 * footer surfaces a live "Matches" summary whenever ≥1 tag is selected.
 */
function TagsSection() {
  const { search, setSearch } = useTaskListSearch();
  const { data: tags } = useTagsWithCounts();
  // Derive untagged count client-side from the page's unfiltered task list
  // (already in flight on /tasks). Free TanStack Query cache hit.
  const { data: allTasks } = useTasks({});
  const untaggedCount = useMemo(
    () => (allTasks ?? []).reduce((n, t) => (t.tags.length === 0 ? n + 1 : n), 0),
    [allTasks],
  );

  const filter = search.tag_filter;
  const selected = useMemo(() => new Set(filter?.tags ?? []), [filter]);
  const mode: TagMatchMode = filter?.mode ?? 'any';
  const untaggedSelected = selected.has(UNTAGGED_TOKEN);

  const [query, setQuery] = useState('');
  const allTags = tags ?? [];
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return allTags;
    return allTags.filter((t) => t.name.toLowerCase().includes(q));
  }, [allTags, query]);
  const showSearch = allTags.length > 8;

  const update = (next: Set<string>, nextMode: TagMatchMode) => {
    const list = Array.from(next);
    setSearch({
      tag_filter: list.length ? { mode: nextMode, tags: list } : undefined,
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

  const clearAll = () => setSearch({ tag_filter: undefined });

  return (
    <Section
      title={
        <div className="flex items-center justify-between gap-2">
          <span>Tags</span>
          <TagModeToggle
            value={mode}
            onChange={setMode}
            allDisabled={untaggedSelected}
            allDisabledReason={UNTAGGED_TOOLTIP}
          />
        </div>
      }
    >
      {showSearch && (
        <Input
          type="search"
          aria-label="Filter tag list"
          placeholder="Filter tags…"
          className="h-7 text-xs"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      )}

      <ul className="flex max-h-[40vh] flex-col gap-px overflow-y-auto">
        {/* Pinned Untagged row — always visible regardless of the filter
            input, since it is a UI affordance rather than a real tag. */}
        <li>
          <TagPickerRow
            name={UNTAGGED_TOKEN}
            selected={untaggedSelected}
            count={untaggedCount}
            onToggle={() => toggle(UNTAGGED_TOKEN)}
          />
        </li>
        <li aria-hidden="true">
          <div className="my-1 border-t border-dashed border-border" />
        </li>

        {allTags.length === 0 ? (
          <li className="px-1 text-xs text-muted-foreground">No tags yet.</li>
        ) : filtered.length === 0 ? (
          <li className="px-1 text-xs text-muted-foreground">No tags match.</li>
        ) : (
          filtered.map((t) => (
            <li key={t.id}>
              <TagPickerRow
                name={t.name}
                selected={selected.has(t.name)}
                count={t.count}
                onToggle={() => toggle(t.name)}
              />
            </li>
          ))
        )}
      </ul>

      {selected.size > 0 && <MatchesSummary mode={mode} selected={selected} onClear={clearAll} />}
    </Section>
  );
}

/**
 * One row in the tag picker — checkbox + color swatch + label + count. The
 * whole row is a button so clicking anywhere toggles selection. Untagged is
 * rendered with a dashed swatch and italic muted label; real tags use the
 * hash-derived hue from `tag-color.ts`.
 */
function TagPickerRow({
  name,
  selected,
  count,
  onToggle,
}: {
  name: string;
  selected: boolean;
  count: number;
  onToggle: () => void;
}) {
  const isUntagged = name === UNTAGGED_TOKEN;
  const displayName = isUntagged ? 'Untagged' : name;
  const ariaLabel = `${selected ? 'Unselect' : 'Select'} tag ${displayName} (${count} task${count === 1 ? '' : 's'})`;
  return (
    <button
      type="button"
      data-tag-name={name}
      data-selected={selected || undefined}
      aria-pressed={selected}
      aria-label={ariaLabel}
      onClick={onToggle}
      className={cn(
        'flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm',
        'hover:bg-accent hover:text-accent-foreground',
        'data-[selected]:bg-accent/60',
      )}
    >
      <Checkbox
        checked={selected}
        tabIndex={-1}
        aria-hidden="true"
        // Visually convey state; clicks are handled by the parent button so
        // the inner checkbox should not steal focus or fire its own change.
        className="pointer-events-none"
      />
      <Swatch name={name} />
      <span className={cn('flex-1 truncate', isUntagged && 'italic text-muted-foreground')}>
        {displayName}
      </span>
      <span className="ml-auto font-mono text-xs tabular-nums text-muted-foreground">{count}</span>
    </button>
  );
}

/**
 * Small color dot used in the picker rows. Mirrors the dot rendered inside
 * `<TagChip>` so the picker and the chips read as the same visual language.
 * Untagged gets a dashed muted ring instead of a filled hue.
 */
function Swatch({ name }: { name: string }) {
  if (name === UNTAGGED_TOKEN) {
    return (
      <span
        aria-hidden="true"
        className="inline-block h-2 w-2 shrink-0 rounded-full border border-dashed border-muted-foreground/60"
      />
    );
  }
  return <RealTagSwatch name={name} />;
}

function RealTagSwatch({ name }: { name: string }) {
  const { resolvedTheme } = useTheme();
  const palette = resolvedTheme === 'dark' ? tagColorDark(name) : tagColor(name);
  return (
    <span
      aria-hidden="true"
      className="inline-block h-2 w-2 shrink-0 rounded-full"
      style={{ backgroundColor: palette.dot }}
    />
  );
}

/**
 * Inline 2-state pill switch for the Tags section's Any/All match mode.
 * The All option becomes disabled (with a tooltip) whenever the Untagged
 * pseudo-tag is selected — Untagged + All is unsatisfiable.
 */
function TagModeToggle({
  value,
  onChange,
  allDisabled,
  allDisabledReason,
}: {
  value: TagMatchMode;
  onChange: (next: TagMatchMode) => void;
  allDisabled?: boolean;
  allDisabledReason?: string;
}) {
  return (
    <TooltipProvider>
      <fieldset
        aria-label="Tag match mode"
        className="inline-flex items-center rounded-md border bg-background p-0.5 text-[10px] font-medium tracking-wide uppercase"
      >
        {(['any', 'all'] as const).map((m) => {
          const active = value === m;
          const disabled = m === 'all' && allDisabled === true;
          const button = (
            <button
              key={m}
              type="button"
              data-tag-mode={m}
              data-active={active || undefined}
              data-disabled={disabled || undefined}
              aria-pressed={active}
              aria-disabled={disabled || undefined}
              disabled={disabled}
              onClick={() => {
                if (disabled) return;
                if (!active) onChange(m);
              }}
              className={cn(
                'rounded px-1.5 py-0.5 transition-colors',
                active
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:text-foreground',
                disabled && 'cursor-not-allowed opacity-50 hover:text-muted-foreground',
              )}
            >
              {m}
            </button>
          );
          if (disabled && allDisabledReason) {
            return (
              <Tooltip key={m}>
                {/* Radix disables pointer events on a disabled <button>,
                    which would also block tooltip hover. Wrap in a span so
                    the pointer-events stay live. */}
                <TooltipTrigger asChild>
                  <span className="inline-flex">{button}</span>
                </TooltipTrigger>
                <TooltipContent>{allDisabledReason}</TooltipContent>
              </Tooltip>
            );
          }
          return button;
        })}
      </fieldset>
    </TooltipProvider>
  );
}

/**
 * Renders the current tag selection as `<TagChip>` chips joined with the
 * mode word (`or` / `and`) so the user can read the resulting query out
 * loud. Includes a Clear link that drops the entire `tag_filter` param.
 */
function MatchesSummary({
  mode,
  selected,
  onClear,
}: {
  mode: TagMatchMode;
  selected: Set<string>;
  onClear: () => void;
}) {
  const items = Array.from(selected);
  const joiner = mode === 'any' ? 'or' : 'and';
  return (
    <div data-testid="matches-summary" className="mt-3 border-t border-dashed border-border pt-3">
      <div className="mb-1.5 flex items-center justify-between">
        <span className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
          Matches
        </span>
        <button
          type="button"
          onClick={onClear}
          className="text-[10px] uppercase tracking-wider text-muted-foreground hover:text-foreground"
        >
          Clear
        </button>
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        {items.map((name, i) => (
          <Fragment key={name}>
            {i > 0 && (
              <span
                data-testid="matches-joiner"
                className="font-mono text-[10px] text-muted-foreground"
              >
                {joiner}
              </span>
            )}
            <TagChip name={name} />
          </Fragment>
        ))}
      </div>
    </div>
  );
}
