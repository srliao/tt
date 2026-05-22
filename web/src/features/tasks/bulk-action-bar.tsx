/**
 * Sticky bottom bar that surfaces bulk actions whenever ≥1 row in the task
 * table is selected. Tag is the primary action (Phase 6 wires the editor);
 * Stage / Mark done / Cancel / Delete are secondary actions with inline
 * keyboard hints. The bar also expands the selection to all matching tasks
 * via "Select all matching · N" when the active filter has unselected rows,
 * and warns about off-screen selections on destructive actions.
 *
 * Keyboard hints (`t` / `s` / `d` / `Esc`) are advertised inline; the
 * shortcuts themselves are wired by the document-level keydown listener
 * in `page.tsx` / Phase 1.
 */

import { useEffect, useMemo, useState } from 'react';
import {
  type TaskListParams,
  useDeleteTask,
  useSetTaskState,
  useStageTask,
  useTasks,
} from '@/api/tasks';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { cn } from '@/lib/utils';
import type { UseSelectionResult } from './use-selection';

export interface BulkActionBarProps {
  selection: UseSelectionResult;
  /** Effective filter currently driving the visible list. Used to compute "select all matching · N". */
  filter: TaskListParams;
  /** Called when the user clicks "Tag…" or presses `t`. Phase 6 wires this to <BulkTagEditor>. */
  onOpenTagEditor: () => void;
}

// kbd hint styled for the dark (inverted) bar surface.
const kbdClass =
  'ml-1 inline-flex items-center justify-center rounded border border-background/25 bg-background/10 px-1 font-mono text-[10px] leading-4';

export function BulkActionBar({ selection, filter, onOpenTagEditor }: BulkActionBarProps) {
  const setState = useSetTaskState();
  const stage = useStageTask();
  const del = useDeleteTask();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmCancel, setConfirmCancel] = useState(false);

  // Resolve "all matching" via the same cached query the list view uses —
  // this is a cache hit when `filter` matches the list page's filter.
  const { data: matching = [] } = useTasks(filter);
  const matchingIds = useMemo(() => new Set(matching.map((t) => t.id)), [matching]);
  const totalMatching = matchingIds.size;
  const allSelected =
    totalMatching > 0 && [...matchingIds].every((id) => selection.selected.has(id));
  const canExpandMatching = totalMatching > 0 && !allSelected;

  // Wire `Esc` to clear the selection when no confirm dialog is open. The
  // AlertDialog primitive consumes Esc itself when open, so this only fires
  // when the bar is the active surface.
  useEffect(() => {
    if (selection.selected.size === 0) return;
    const handler = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      if (confirmDelete || confirmCancel) return;
      // Don't hijack Esc out of inputs (e.g. command palette / inline editors).
      const target = event.target as HTMLElement | null;
      if (target) {
        const tag = target.tagName;
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
        if (target.isContentEditable) return;
      }
      event.preventDefault();
      selection.clear();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [selection, confirmDelete, confirmCancel]);

  if (selection.selected.size === 0) return null;
  const ids = Array.from(selection.selected);

  const markDone = () => {
    for (const id of ids) {
      setState.mutate({ id, state: 'done' });
    }
    selection.clear();
  };

  const stageAll = () => {
    for (const id of ids) {
      stage.mutate(id);
    }
    selection.clear();
  };

  const cancelAll = () => {
    for (const id of ids) {
      setState.mutate({ id, state: 'cancelled' });
    }
    setConfirmCancel(false);
    selection.clear();
  };

  const deleteAll = () => {
    for (const id of ids) {
      del.mutate(id);
    }
    setConfirmDelete(false);
    selection.clear();
  };

  const expandToMatching = () => {
    selection.add(matchingIds);
  };

  return (
    <section
      aria-label="Bulk actions"
      className="fixed bottom-4 left-1/2 z-30 flex w-fit -translate-x-1/2 items-center gap-2.5 rounded-2xl bg-foreground px-3 py-2 text-background text-sm shadow-xl ring-1 ring-foreground/10"
    >
      {/* Counter */}
      <span className="inline-flex items-center gap-1.5 rounded-full bg-background/10 px-2.5 py-0.5 font-mono text-[11px]">
        <b className="font-semibold">{selection.selected.size}</b>
        <span className="opacity-70">selected</span>
        {selection.offScreenCount > 0 && (
          <>
            <span className="opacity-40">·</span>
            <b className="font-semibold">{selection.offScreenCount}</b>
            <span className="opacity-70">off-screen</span>
          </>
        )}
      </span>

      <span className="h-4 w-px bg-background/20" aria-hidden="true" />

      {/* Primary action */}
      <button
        type="button"
        onClick={onOpenTagEditor}
        className="inline-flex items-center rounded-md bg-background px-2.5 py-1 font-medium text-foreground text-sm shadow-sm transition-colors hover:bg-background/90"
      >
        Tag…
        <kbd className={kbdClass}>t</kbd>
      </button>

      {/* Secondary actions */}
      <button
        type="button"
        onClick={stageAll}
        className="inline-flex items-center rounded-md px-2 py-1 text-sm transition-colors hover:bg-background/10"
      >
        Stage
        <kbd className={kbdClass}>s</kbd>
      </button>
      <button
        type="button"
        onClick={markDone}
        className="inline-flex items-center rounded-md px-2 py-1 text-sm transition-colors hover:bg-background/10"
      >
        Mark done
        <kbd className={kbdClass}>d</kbd>
      </button>
      <button
        type="button"
        onClick={() => setConfirmCancel(true)}
        className="inline-flex items-center rounded-md px-2 py-1 text-sm transition-colors hover:bg-background/10"
      >
        Cancel
      </button>
      <button
        type="button"
        onClick={() => setConfirmDelete(true)}
        className="inline-flex items-center rounded-md px-2 py-1 text-red-300 text-sm transition-colors hover:bg-red-500/20 hover:text-red-200"
      >
        Delete
      </button>

      {/* Meta + clear */}
      <span className={cn('ml-3 flex items-center gap-3 text-[12px] opacity-70')}>
        {canExpandMatching && (
          <button
            type="button"
            onClick={expandToMatching}
            className="rounded px-1 underline-offset-2 hover:underline"
          >
            Select all matching · {totalMatching}
          </button>
        )}
        <span className="h-4 w-px bg-background/20" aria-hidden="true" />
        <button
          type="button"
          onClick={() => selection.clear()}
          className="inline-flex items-center rounded px-1 hover:underline"
        >
          Clear
          <kbd className={kbdClass}>Esc</kbd>
        </button>
      </span>

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {selection.selected.size} task(s)?</AlertDialogTitle>
            <AlertDialogDescription>
              This will delete <b>{selection.selected.size} tasks</b>
              {selection.offScreenCount > 0 && (
                <>
                  {' '}
                  — including <b>{selection.offScreenCount}</b> not visible under the current filter
                </>
              )}
              . This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={deleteAll}>
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmCancel} onOpenChange={setConfirmCancel}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel {selection.selected.size} task(s)?</AlertDialogTitle>
            <AlertDialogDescription>
              This will cancel <b>{selection.selected.size} tasks</b>
              {selection.offScreenCount > 0 && (
                <>
                  {' '}
                  — including <b>{selection.offScreenCount}</b> not visible under the current filter
                </>
              )}
              .
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep tasks</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={cancelAll}>
              Cancel tasks
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
