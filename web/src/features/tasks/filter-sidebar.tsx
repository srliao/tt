/**
 * Left sidebar on the /tasks page: quick filters, state checkboxes, tag
 * multi-select, due-range select, debounced search input.
 *
 * All UI state lives in the URL via `useTaskListSearch()` so refreshes and
 * shared links stay stable. The only locally-held value is the debounced
 * search input — we keep it as React state until 300ms after the last edit,
 * then push to the URL.
 */

import { ChevronDownIcon, SearchIcon, XIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTags } from '@/api/tags';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import type { TaskDueRange, TaskState } from '@/types/task';
import {
  QUICK_FILTERS,
  type QuickFilter,
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

      <Section title="Tags">
        <TagMultiSelect
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

function Section({ title, children }: { title: string; children: React.ReactNode }) {
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

function TagMultiSelect({
  value,
  onChange,
}: {
  value: string[];
  onChange: (next: string[]) => void;
}) {
  const { data: tags } = useTags();
  const [open, setOpen] = useState(false);
  const toggle = (name: string, checked: boolean) => {
    const set = new Set(value);
    if (checked) set.add(name);
    else set.delete(name);
    onChange(Array.from(set));
  };

  return (
    <div className="flex flex-col gap-2">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button variant="outline" className="w-full justify-between" type="button">
            <span className="truncate text-left">
              {value.length === 0
                ? 'Any tag'
                : value.length === 1
                  ? value[0]
                  : `${value.length} selected`}
            </span>
            <ChevronDownIcon className="opacity-60" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-60 p-2" align="start">
          <div className="max-h-64 overflow-y-auto">
            {(tags ?? []).length === 0 && (
              <p className="px-2 py-1 text-xs text-muted-foreground">No tags yet.</p>
            )}
            {(tags ?? []).map((t) => {
              const id = `tag-${t.id}`;
              return (
                <div
                  key={t.id}
                  className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-accent"
                >
                  <Checkbox
                    id={id}
                    checked={value.includes(t.name)}
                    onCheckedChange={(c) => toggle(t.name, c === true)}
                  />
                  <label htmlFor={id} className="cursor-pointer">
                    {t.name}
                  </label>
                </div>
              );
            })}
          </div>
        </PopoverContent>
      </Popover>
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {value.map((name) => (
            <Badge key={name} variant="secondary" className="gap-0.5 pr-1">
              {name}
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                aria-label={`Remove tag ${name}`}
                onClick={() => toggle(name, false)}
              >
                <XIcon />
              </Button>
            </Badge>
          ))}
        </div>
      )}
    </div>
  );
}
