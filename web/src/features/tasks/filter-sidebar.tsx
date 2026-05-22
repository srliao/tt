/**
 * Left sidebar on the /tasks page: quick filters, state checkboxes, tag
 * multi-select, due-range select, debounced search input.
 *
 * All UI state lives in the URL via `useTaskListSearch()` so refreshes and
 * shared links stay stable. The only locally-held value is the debounced
 * search input — we keep it as React state until 300ms after the last edit,
 * then push to the URL.
 */

import { CheckIcon, SearchIcon } from 'lucide-react';
import { type ReactNode, useEffect, useMemo, useState } from 'react';
import { useTagsWithCounts } from '@/api/tags';
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
import { cn } from '@/lib/utils';
import type { TaskDueRange, TaskState } from '@/types/task';
import {
  QUICK_FILTERS,
  type QuickFilter,
  type TagMode,
  type TaskSearch,
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

function isStateRestricted(search: TaskSearch): boolean {
  if (search.states && search.states.length > 0) {
    return !(search.states.length === 3);
  }
  // Default is `not_done` only.
  return true;
}

export function FilterSidebar() {
  const { search, setSearch } = useTaskListSearch();
  const [searchInput, setSearchInput] = useState(search.q ?? '');

  // Keep the local input in sync when the URL is mutated externally
  // (e.g. quick-filter click). Compare against the URL value, not the
  // previous local value, so we only overwrite when the URL changed.
  useEffect(() => {
    setSearchInput(search.q ?? '');
  }, [search.q]);

  // Debounced push to URL.
  useEffect(() => {
    const t = setTimeout(() => {
      const current = search.q ?? '';
      if (searchInput !== current) {
        setSearch({ q: searchInput || undefined });
      }
    }, 300);
    return () => clearTimeout(t);
  }, [searchInput, search.q, setSearch]);

  return (
    <aside className="flex w-60 shrink-0 flex-col gap-4 p-4 text-sm">
      <SearchField value={searchInput} onChange={setSearchInput} />
      {isStateRestricted(search) && (
        <p className="-mt-2 text-xs text-muted-foreground">Searching open tasks only.</p>
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

      <Section
        title={
          <div className="flex items-center justify-between gap-2">
            <span>Tags</span>
            <TagModeToggle
              value={search.tagMode ?? 'any'}
              onChange={(m) => setSearch({ tagMode: m === 'any' ? undefined : m })}
            />
          </div>
        }
      >
        <TagInlineList
          value={search.tags ?? []}
          onChange={(tags) => setSearch({ tags: tags.length ? tags : undefined })}
        />
      </Section>

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

function SearchField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div className="relative">
      <SearchIcon
        className="pointer-events-none absolute top-1/2 left-2 size-4 -translate-y-1/2 text-muted-foreground"
        aria-hidden="true"
      />
      <Input
        type="search"
        data-search-input
        placeholder="Search tasks…"
        aria-label="Search tasks"
        className="pl-8"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
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
 * Inline 2-state pill switch for the tag any/all toggle. The user-facing
 * default is `any`; only `all` is written to the URL by the caller, but the
 * component itself always shows a non-null value so the active mode is
 * obvious at a glance.
 */
function TagModeToggle({ value, onChange }: { value: TagMode; onChange: (next: TagMode) => void }) {
  return (
    <fieldset
      aria-label="Tag match mode"
      className="inline-flex items-center rounded-md border bg-background p-0.5 text-[10px] font-medium tracking-wide uppercase"
    >
      {(['any', 'all'] as const).map((m) => {
        const active = value === m;
        return (
          <button
            key={m}
            type="button"
            data-tag-mode={m}
            data-active={active || undefined}
            aria-pressed={active}
            onClick={() => {
              if (!active) onChange(m);
            }}
            className={cn(
              'rounded px-1.5 py-0.5 transition-colors',
              active
                ? 'bg-accent text-accent-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {m}
          </button>
        );
      })}
    </fieldset>
  );
}

/**
 * Inline tag list — always visible, no popover. Selected chips are pinned
 * at the top so the user can scan + remove them quickly, with the full
 * tag inventory (and per-tag counts) below. A search-in-list input appears
 * once the inventory grows past 8 entries.
 */
function TagInlineList({
  value,
  onChange,
}: {
  value: string[];
  onChange: (next: string[]) => void;
}) {
  const { data: tags } = useTagsWithCounts();
  const [query, setQuery] = useState('');

  const allTags = tags ?? [];
  const selected = useMemo(() => new Set(value), [value]);

  const toggle = (name: string) => {
    const set = new Set(value);
    if (set.has(name)) set.delete(name);
    else set.add(name);
    onChange(Array.from(set));
  };

  const removeTag = (name: string) => {
    if (!selected.has(name)) return;
    onChange(value.filter((v) => v !== name));
  };

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return allTags;
    return allTags.filter((t) => t.name.toLowerCase().includes(q));
  }, [allTags, query]);

  const showSearch = allTags.length > 8;

  return (
    <div className="flex flex-col gap-2">
      {value.length > 0 && (
        <div
          data-testid="selected-tag-chips"
          className="flex flex-wrap gap-1 rounded-md border bg-background p-1.5"
        >
          {value.map((name) => (
            <TagChip key={name} name={name} variant="outline" onRemove={() => removeTag(name)} />
          ))}
        </div>
      )}

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

      {allTags.length === 0 ? (
        <p className="px-1 text-xs text-muted-foreground">No tags yet.</p>
      ) : filtered.length === 0 ? (
        <p className="px-1 text-xs text-muted-foreground">No tags match.</p>
      ) : (
        <ul className="flex max-h-[40vh] flex-col gap-px overflow-y-auto">
          {filtered.map((t) => {
            const isSelected = selected.has(t.name);
            return (
              <li key={t.id}>
                <button
                  type="button"
                  data-tag-name={t.name}
                  data-selected={isSelected || undefined}
                  aria-pressed={isSelected}
                  aria-label={`${isSelected ? 'Unselect' : 'Select'} tag ${t.name} (${t.count} task${t.count === 1 ? '' : 's'})`}
                  onClick={() => toggle(t.name)}
                  className={cn(
                    'flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left text-sm',
                    'hover:bg-accent hover:text-accent-foreground',
                    'data-[selected]:bg-accent/60',
                  )}
                >
                  <span
                    aria-hidden="true"
                    data-checked={isSelected || undefined}
                    className={cn(
                      'flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-input',
                      'data-[checked]:border-primary data-[checked]:bg-primary data-[checked]:text-primary-foreground',
                    )}
                  >
                    {isSelected && <CheckIcon className="size-3" />}
                  </span>
                  <TagChip name={t.name} variant="outline" size="sm" />
                  <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                    {t.count}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
