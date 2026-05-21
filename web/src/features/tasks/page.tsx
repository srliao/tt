/**
 * The /tasks page. Wires together:
 *
 * - `FilterSidebar` (URL-driven filters/search) — left column.
 * - `TaskTable` (data display + dnd-kit reorder + j/k shortcuts) — right column.
 * - `AddTaskModal` (controlled here so the global `n` shortcut can open it).
 * - `BulkActionBar` (sticky footer that appears when ≥1 row is selected).
 *
 * The empty state per spec §6 only shows when (a) the server returned zero
 * rows AND (b) no filter is active. Otherwise we render the table even when
 * empty so the user can see their filter is doing something.
 */

import { Link } from '@tanstack/react-router';
import { PlusIcon } from 'lucide-react';
import { useState } from 'react';
import { useTasks } from '@/api/tasks';
import { Button } from '@/components/ui/button';
import type { Task } from '@/types/task';
import { AddTaskModal, useNewTaskListener } from './add-task-modal';
import { BulkActionBar } from './bulk-action-bar';
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

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());

  useNewTaskListener(() => setModalOpen(true));

  const filtersActive = hasActiveFilters(search);
  const showEmpty = !isLoading && tasks.length === 0 && !filtersActive;

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)]">
      <FilterSidebar />
      <section className="flex-1 px-4 py-4">
        <header className="mb-3 flex items-center justify-between">
          <h1 className="font-heading text-xl font-medium">Tasks</h1>
          <Button size="sm" onClick={() => setModalOpen(true)}>
            <PlusIcon /> New task
          </Button>
        </header>

        {showEmpty ? (
          <EmptyState onCreate={() => setModalOpen(true)} />
        ) : (
          <TaskTable
            tasks={tasks}
            sort={sort}
            selectedIds={selectedIds}
            onSelectedChange={setSelectedIds}
            onEdit={(t) => setEditing(t)}
            hasFilters={filtersActive}
          />
        )}
      </section>

      <BulkActionBar selectedIds={selectedIds} onClear={() => setSelectedIds(new Set())} />

      <AddTaskModal open={modalOpen} onOpenChange={setModalOpen} />

      {/*
       * Edit modal is wired through the same Add modal for v1 — phase 08e
       * will introduce a dedicated edit dialog. Until then, surface the
       * edited task via a no-op until then.
       */}
      {editing && (
        <p className="sr-only" aria-live="polite">
          Editing task {editing.title}
        </p>
      )}
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
