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
import { CheckSquareIcon, PlusIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTasks } from '@/api/tasks';
import { Button } from '@/components/ui/button';
import type { Task } from '@/types/task';
import { AddTaskModal, useNewTaskListener } from './add-task-modal';
import { BulkActionBar } from './bulk-action-bar';
import { EditTaskModal } from './edit-task-modal';
import { FilterSidebar } from './filter-sidebar';
import { TaskTable } from './task-table';
import { applyQuickFilter, hasActiveFilters, useTaskListSearch } from './use-task-list-search';

export function TasksPage() {
  const { search } = useTaskListSearch();
  const effective = applyQuickFilter(search);
  // Default sort is `priority` so the drag-handle column is visible until the
  // user explicitly picks another sort axis.
  const sort = effective.sort ?? 'priority';

  const { data: tasks = [], isLoading } = useTasks({ ...effective, sort });

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [multiSelectMode, setMultiSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());

  useNewTaskListener(() => setCreating(true));

  // The global command palette dispatches `tt:open-task` with a task id when
  // the user picks a task result. We open the edit modal for that task if
  // it's in the currently-loaded list. Mirrors `useNewTaskListener` above.
  useEffect(() => {
    const handler = (event: Event) => {
      const id = (event as CustomEvent<{ id: number }>).detail?.id;
      if (typeof id !== 'number') return;
      const target = tasks.find((t) => t.id === id);
      if (target) setEditing(target);
    };
    window.addEventListener('tt:open-task', handler);
    return () => window.removeEventListener('tt:open-task', handler);
  }, [tasks]);

  const filtersActive = hasActiveFilters(search);
  const showEmpty = !isLoading && tasks.length === 0 && !filtersActive;

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)]">
      <FilterSidebar />
      <section className="flex-1 px-4 py-4">
        <header className="mb-3 flex items-center justify-between">
          <h1 className="font-heading text-xl font-medium">Tasks</h1>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant={multiSelectMode ? 'secondary' : 'outline'}
              aria-pressed={multiSelectMode}
              onClick={() => {
                setMultiSelectMode((v) => {
                  if (v) setSelectedIds(new Set());
                  return !v;
                });
              }}
            >
              <CheckSquareIcon /> Multi-select
            </Button>
            <Button size="sm" onClick={() => setCreating(true)}>
              <PlusIcon /> New task
            </Button>
          </div>
        </header>

        {showEmpty ? (
          <EmptyState onCreate={() => setCreating(true)} />
        ) : (
          <TaskTable
            tasks={tasks}
            sort={sort}
            multiSelectMode={multiSelectMode}
            selectedIds={selectedIds}
            onSelectedChange={setSelectedIds}
            onEdit={(t) => setEditing(t)}
            hasFilters={filtersActive}
          />
        )}
      </section>

      {multiSelectMode && (
        <BulkActionBar selectedIds={selectedIds} onClear={() => setSelectedIds(new Set())} />
      )}

      <AddTaskModal open={creating} onOpenChange={setCreating} />

      <EditTaskModal
        task={editing}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
      />
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
