/**
 * Global command palette. Opens with `/` (when not typing in an input) or
 * `⌘K` / `Ctrl+K`. Fuzzy-searches tasks and tags, lets the user apply the
 * typed query as a `?q=` filter via `⌘↵`, and provides a Go-to list for
 * the five top-level pages.
 *
 * Selecting a task navigates to `/tasks?open=<id>`. The /tasks page watches
 * that search-param and opens the edit modal. This avoids the race that the
 * previous CustomEvent-based mechanism had: under concurrent rendering the
 * listener may not be installed by the time a dispatched event fires.
 *
 * The palette installs its own keydown listener. The legacy `/` →
 * focus-search handler in `useGlobalShortcuts` has been removed; this
 * component now owns that key.
 */

import { useNavigate } from '@tanstack/react-router';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTagsWithCounts } from '@/api/tags';
import { useTasks } from '@/api/tasks';
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command';
import { TagChip } from '@/components/ui/tag-chip';

const NAV: ReadonlyArray<{
  label: string;
  to: '/stage' | '/tasks' | '/scripts' | '/tags' | '/runs';
}> = [
  { label: 'Stage', to: '/stage' },
  { label: 'Tasks', to: '/tasks' },
  { label: 'Scripts', to: '/scripts' },
  { label: 'Tags', to: '/tags' },
  { label: 'Runs', to: '/runs' },
];

function isTypingTarget(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false;
  const tag = t.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
  if (t.isContentEditable) return true;
  return false;
}

export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const navigate = useNavigate();

  // Empty params ⇒ the server returns the full unfiltered list, which is
  // what the palette should search.
  const { data: tasks = [] } = useTasks({});
  const { data: tags = [] } = useTagsWithCounts();

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      const key = e.key;
      const cmdK = (e.metaKey || e.ctrlKey) && key.toLowerCase() === 'k';
      // Don't capture `/` when the user is typing somewhere. cmdK is captured
      // unconditionally (modifier keys never collide with normal text entry).
      const slash = key === '/' && !isTypingTarget(e.target);
      if (cmdK || slash) {
        e.preventDefault();
        setOpen((prev) => !prev);
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, []);

  // Reset the query when the palette closes so the next open starts fresh.
  // Best-effort focus restore: hand keyboard control back to the task table
  // if it's currently in the DOM.
  useEffect(() => {
    if (!open) {
      setQuery('');
      const table = document.querySelector<HTMLElement>('[data-task-table]');
      table?.focus();
    }
  }, [open]);

  const q = query.trim().toLowerCase();

  const matchedTasks = useMemo(() => {
    if (!q) return [];
    return tasks.filter((t) => t.title.toLowerCase().includes(q)).slice(0, 6);
  }, [tasks, q]);

  const matchedTags = useMemo(() => {
    if (!q) return [];
    return tags.filter((t) => t.name.toLowerCase().includes(q)).slice(0, 8);
  }, [tags, q]);

  const applyQueryAsFilter = useCallback(
    (text: string) => {
      const value = text.trim();
      void navigate({
        to: '/tasks',
        search: (prev) => ({
          ...(prev as Record<string, unknown>),
          q: value || undefined,
        }),
      });
      setOpen(false);
    },
    [navigate],
  );

  const filterByTag = useCallback(
    (name: string) => {
      void navigate({
        to: '/tasks',
        search: (prev) => ({
          ...(prev as Record<string, unknown>),
          tags: [name],
          tagsExclude: undefined,
        }),
      });
      setOpen(false);
    },
    [navigate],
  );

  const openTask = useCallback(
    (id: number) => {
      // Encode the request as a URL search-param signal. The /tasks page
      // watches `search.open`, opens the edit modal, and clears the field.
      // Race-free: the URL is settled before the page commits.
      void navigate({
        to: '/tasks',
        search: (prev) => ({ ...(prev as Record<string, unknown>), open: id }),
      });
      setOpen(false);
    },
    [navigate],
  );

  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      title="Command palette"
      description="Search tasks, tags, or jump to a page."
    >
      <CommandInput
        placeholder="Search tasks, tags, or jump to…"
        value={query}
        onValueChange={setQuery}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault();
            applyQueryAsFilter(query);
          }
        }}
      />
      <CommandList>
        <CommandEmpty>No matches.</CommandEmpty>

        {q && (
          <>
            <CommandGroup heading={`Tasks · ${matchedTasks.length}`}>
              {matchedTasks.map((t) => (
                <CommandItem
                  key={`task-${t.id}`}
                  value={`task-${t.id}-${t.title}`}
                  onSelect={() => openTask(t.id)}
                >
                  <span className="truncate">{highlight(t.title, query)}</span>
                  {t.due_date && (
                    <span className="ml-auto text-xs text-muted-foreground">{t.due_date}</span>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>

            <CommandGroup heading="Filter">
              <CommandItem value={`__filter-${query}`} onSelect={() => applyQueryAsFilter(query)}>
                <span>
                  Show only tasks matching <b className="ml-1">{query}</b>
                </span>
                <kbd className="ml-auto rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                  ⌘ ↵
                </kbd>
              </CommandItem>
            </CommandGroup>

            {matchedTags.length > 0 && (
              <>
                <CommandSeparator />
                <CommandGroup heading={`Tags · ${matchedTags.length}`}>
                  {matchedTags.map((t) => (
                    <CommandItem
                      key={`tag-${t.id}`}
                      value={`tag-${t.name}`}
                      onSelect={() => filterByTag(t.name)}
                    >
                      <TagChip name={t.name} />
                      <span className="ml-auto text-xs text-muted-foreground">filter by tag</span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </>
            )}
          </>
        )}

        <CommandSeparator />
        <CommandGroup heading="Go to">
          {NAV.map((n) => (
            <CommandItem
              key={n.to}
              value={`nav-${n.to}`}
              onSelect={() => {
                void navigate({ to: n.to });
                setOpen(false);
              }}
            >
              {n.label}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}

function highlight(text: string, q: string) {
  if (!q) return text;
  const i = text.toLowerCase().indexOf(q.toLowerCase());
  if (i === -1) return text;
  return (
    <>
      {text.slice(0, i)}
      <mark className="rounded-sm bg-yellow-200/60 px-0.5">{text.slice(i, i + q.length)}</mark>
      {text.slice(i + q.length)}
    </>
  );
}
