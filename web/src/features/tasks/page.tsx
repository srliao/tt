/**
 * The /tasks page. Wires together:
 *
 * - `FilterSidebar` (URL-driven filters/search) — left column.
 * - `TaskTable` (data display + dnd-kit reorder + j/k shortcuts) — right column.
 * - `AddTaskModal` (title-only create modal; opened by the header button or
 *   the global `n` shortcut).
 * - `EditTaskModal` (full edit surface; opened by clicking a task title or
 *   pressing `e`).
 * - `BulkActionBar` (sticky footer that appears when ≥1 row is selected).
 *
 * The empty state per spec §6 only shows when (a) the server returned zero
 * rows AND (b) no filter is active. Otherwise we render the table even when
 * empty so the user can see their filter is doing something.
 */

import { Link } from '@tanstack/react-router';
import { PlusIcon } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTasks } from '@/api/tasks';
import { Button } from '@/components/ui/button';
import type { Task } from '@/types/task';
import { ActiveFilterStrip } from './active-filter-strip';
import { AddTaskModal, useNewTaskListener } from './add-task-modal';
import { BulkActionBar } from './bulk-action-bar';
import { BulkTagEditor } from './bulk-tag-editor';
import { EditTaskModal } from './edit-task-modal';
import { FilterSidebar } from './filter-sidebar';
import { InlineTagEditor } from './inline-tag-editor';
import { TaskTable } from './task-table';
import { useSelection } from './use-selection';
import {
  applyQuickFilter,
  computeAllMatchingIds,
  hasActiveFilters,
  useTaskListSearch,
} from './use-task-list-search';

export function TasksPage() {
  const { search, setSearch } = useTaskListSearch();
  const effective = applyQuickFilter(search);
  // Default sort is `priority` so the drag-handle column is visible until the
  // user explicitly picks another sort axis.
  const sort = effective.sort ?? 'priority';

  const { data: tasks = [], isLoading } = useTasks({ ...effective, sort });
  // Unfiltered list used to resolve `?open=<id>` requests from the command
  // palette. The palette caches this same query key, so this is usually a
  // free cache hit when arriving from the palette. Falls back to the
  // filtered list when both are populated.
  const { data: allTasks, isFetched: allTasksFetched } = useTasks({});

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [editingTags, setEditingTags] = useState<Task | null>(null);
  const [bulkTagOpen, setBulkTagOpen] = useState(false);
  const tagButtonRef = useRef<HTMLButtonElement | null>(null);
  const selection = useSelection(tasks);

  // Resolve selected tasks against the full (unfiltered) list so off-screen
  // selections still surface their tag data in the bulk editor. Falls back to
  // the visible list while `allTasks` is loading.
  const selectedTasks = useMemo(() => {
    const source = allTasks ?? tasks;
    return source.filter((t) => selection.selected.has(t.id));
  }, [allTasks, tasks, selection.selected]);

  useNewTaskListener(() => setCreating(true));

  // Guard against a stale `bulkTagOpen=true` if selection is cleared from
  // outside the editor (e.g. another tab mutating tasks away, programmatic
  // deselection). The popover already short-circuits to `null` when the
  // selection is empty, but `shortcutsDisabled` still depends on this flag,
  // so table shortcuts would silently stay disabled until reopen.
  useEffect(() => {
    if (selection.selected.size === 0 && bulkTagOpen) setBulkTagOpen(false);
  }, [selection.selected.size, bulkTagOpen]);

  // The command palette navigates here with `?open=<id>` to request the edit
  // modal. We resolve the id against the unfiltered task list (so it works
  // regardless of the current filters), open the modal, and immediately
  // clear `open` so refresh/back doesn't reopen indefinitely. If the id no
  // longer exists, silently clear it — best-effort UX, no toast.
  const openId = search.open;
  useEffect(() => {
    if (typeof openId !== 'number') return;
    // Wait for the unfiltered list to resolve before deciding the id is
    // missing — otherwise a fresh page load with `?open=` would clear the
    // signal before the data arrives.
    if (!allTasksFetched) return;
    const target =
      (allTasks ?? []).find((t) => t.id === openId) ?? tasks.find((t) => t.id === openId);
    if (target) setEditing(target);
    setSearch({ open: undefined });
  }, [openId, allTasks, allTasksFetched, tasks, setSearch]);

  const filtersActive = hasActiveFilters(search);
  const showEmpty = !isLoading && tasks.length === 0 && !filtersActive;

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)]">
      <FilterSidebar />
      <section className="flex-1 px-4 py-4">
        <header className="mb-3 flex items-center justify-between">
          <h1 className="font-heading text-xl font-medium">Tasks</h1>
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={() => setCreating(true)}>
              <PlusIcon /> New task
            </Button>
          </div>
        </header>

        <ActiveFilterStrip />

        {showEmpty ? (
          <EmptyState onCreate={() => setCreating(true)} />
        ) : (
          <TaskTable
            tasks={tasks}
            sort={sort}
            selectedIds={selection.selected}
            onSelectedChange={(next) => selection.setAll(next)}
            onEdit={(t) => setEditing(t)}
            onEditTags={(t) => setEditingTags(t)}
            onOpenBulkTagEditor={() => setBulkTagOpen(true)}
            shortcutsDisabled={editingTags !== null || bulkTagOpen}
            hasFilters={filtersActive}
            onSelectAllMatching={() => {
              // The palette-open feature already loads `useTasks({})`, so this
              // is normally a free cache hit. If it hasn't resolved yet,
              // no-op — the user can press the shortcut again after load.
              const all = allTasks ?? [];
              if (all.length === 0) return;
              const ids = computeAllMatchingIds(all, { ...effective, sort });
              const target = new Set(ids);
              // Toggle: if the current selection already equals the target
              // set, clear instead. Mirrors the visible-toggle behaviour of
              // ⌘A so both shortcuts double as deselectors.
              const sameSize = target.size === selection.selected.size;
              const alreadyAll = sameSize && [...target].every((id) => selection.selected.has(id));
              if (alreadyAll) selection.setAll(new Set());
              else selection.setAll(target);
            }}
          />
        )}
      </section>

      {selection.selected.size > 0 && (
        <BulkActionBar
          selection={selection}
          filter={{ ...effective, sort }}
          onOpenTagEditor={() => setBulkTagOpen(true)}
          tagButtonRef={tagButtonRef}
        />
      )}

      <BulkTagEditor
        selectedTasks={selectedTasks}
        open={bulkTagOpen && selection.selected.size > 0}
        onOpenChange={setBulkTagOpen}
        anchorRef={tagButtonRef}
      />

      <AddTaskModal open={creating} onOpenChange={setCreating} />

      <EditTaskModal
        task={editing}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
      />

      <InlineTagEditor task={editingTags} onClose={() => setEditingTags(null)} />
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-12 text-center">
      <h2 className="font-heading text-lg">No tasks yet</h2>
      <p className="max-w-md text-sm text-muted-foreground">
        Tasks are your unit of work. Stage a batch you want to focus on, then run scripts against
        them — see{' '}
        <Link to="/scripts" className="underline">
          Scripts
        </Link>{' '}
        for automation, or add your first task to get started.
      </p>
      <Button onClick={onCreate}>
        <PlusIcon /> Create your first task
      </Button>
    </div>
  );
}
