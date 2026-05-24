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

import { type RefObject, useMemo, useState } from 'react';
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

/**
 * Controlled-pair for a confirm dialog. The bar treats the dialog as
 * controlled only when BOTH the value and the setter are provided. Passing
 * exactly one is a foot-gun — we'd read from the prop but write to the
 * no-op internal setter, resulting in a partially-controlled dialog that
 * never closes. Modeling each pair as a discriminated union surfaces the
 * mistake at the type level; we also warn at runtime in dev (see below).
 */
type DeleteControl =
  | { confirmDelete?: undefined; onConfirmDeleteChange?: undefined }
  | { confirmDelete: boolean; onConfirmDeleteChange: (next: boolean) => void };

type CancelControl =
  | { confirmCancel?: undefined; onConfirmCancelChange?: undefined }
  | { confirmCancel: boolean; onConfirmCancelChange: (next: boolean) => void };

type BulkActionBarBaseProps = {
  selection: UseSelectionResult;
  /** Effective filter currently driving the visible list. Used to compute "select all matching · N". */
  filter: TaskListParams;
  /** Called when the user clicks "Tag…" or presses `t`. Phase 6 wires this to <BulkTagEditor>. */
  onOpenTagEditor: () => void;
  /**
   * Forwarded to the "Tag…" button so <BulkTagEditor> can anchor its popover
   * to it. Optional because callers that don't render the editor (tests,
   * legacy consumers) shouldn't have to provide one.
   */
  tagButtonRef?: RefObject<HTMLButtonElement | null>;
};

export type BulkActionBarProps = BulkActionBarBaseProps & DeleteControl & CancelControl;

// kbd hint styled for the dark (inverted) bar surface.
const kbdClass =
  'ml-1 inline-flex items-center justify-center rounded border border-background/25 bg-background/10 px-1 font-mono text-[10px] leading-4';

// Keyboard-focus ring tuned for the dark bar surface; the default browser
// outline disappears against `bg-foreground`.
const focusRing =
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-background/50';

// Primary (inverted/light) button on the dark bar.
const primaryBtn = cn(
  'inline-flex items-center rounded-md bg-background px-2.5 py-1 font-medium text-foreground text-sm shadow-sm transition-colors hover:bg-background/90',
  focusRing,
);
// Secondary (ghost) action button on the dark bar.
const secondaryBtn = cn(
  'inline-flex items-center rounded-md px-2 py-1 text-sm transition-colors hover:bg-background/10',
  focusRing,
);
// Destructive (red ghost) action button on the dark bar.
const destructiveBtn = cn(
  'inline-flex items-center rounded-md px-2 py-1 text-red-300 text-sm transition-colors hover:bg-red-500/20 hover:text-red-200',
  focusRing,
);
// Small meta-row action ("Select all matching", "Clear").
const metaBtn = cn('rounded px-1 hover:underline', focusRing);

export function BulkActionBar({
  selection,
  filter,
  onOpenTagEditor,
  tagButtonRef,
  confirmDelete: confirmDeleteProp,
  onConfirmDeleteChange,
  confirmCancel: confirmCancelProp,
  onConfirmCancelChange,
}: BulkActionBarProps) {
  const setState = useSetTaskState();
  const stage = useStageTask();
  const del = useDeleteTask();
  // Fall back to internal state when the parent doesn't control the dialogs
  // (test consumers, future call sites that don't need palette-driven opens).
  // A dialog is treated as controlled only when BOTH the value and the
  // setter are provided. Partial control would silently desync (we'd read
  // from the prop but write to the no-op internal setter, leaving the
  // dialog stuck open). The discriminated union on the props type
  // prevents this at compile-time; the dev warning below is a belt-and-
  // suspenders runtime check for JS callers / loose any.
  const [confirmDeleteLocal, setConfirmDeleteLocal] = useState(false);
  const [confirmCancelLocal, setConfirmCancelLocal] = useState(false);
  if (import.meta.env.DEV) {
    if ((confirmDeleteProp === undefined) !== (onConfirmDeleteChange === undefined)) {
      console.warn(
        'BulkActionBar: confirmDelete and onConfirmDeleteChange must be passed together (both controlled or both omitted).',
      );
    }
    if ((confirmCancelProp === undefined) !== (onConfirmCancelChange === undefined)) {
      console.warn(
        'BulkActionBar: confirmCancel and onConfirmCancelChange must be passed together (both controlled or both omitted).',
      );
    }
  }
  const isDeleteControlled = confirmDeleteProp !== undefined && onConfirmDeleteChange !== undefined;
  const isCancelControlled = confirmCancelProp !== undefined && onConfirmCancelChange !== undefined;
  const confirmDelete = isDeleteControlled ? confirmDeleteProp : confirmDeleteLocal;
  const setConfirmDelete = isDeleteControlled ? onConfirmDeleteChange : setConfirmDeleteLocal;
  const confirmCancel = isCancelControlled ? confirmCancelProp : confirmCancelLocal;
  const setConfirmCancel = isCancelControlled ? onConfirmCancelChange : setConfirmCancelLocal;

  // Resolve "all matching" via the same cached query the list view uses —
  // this is a cache hit when `filter` matches the list page's filter.
  // NOTE: `matchingIds` reflects the result of useTasks(filter); page.tsx does
  // not paginate today, so totalMatching equals the global filtered count. If
  // pagination is added, switch to a server-side count endpoint.
  const { data: matching = [] } = useTasks(filter);
  const matchingIds = useMemo(() => new Set(matching.map((t) => t.id)), [matching]);
  const totalMatching = matchingIds.size;
  const allSelected =
    totalMatching > 0 && [...matchingIds].every((id) => selection.selected.has(id));
  const canExpandMatching = totalMatching > 0 && !allSelected;

  // Esc-to-clear is handled by `useTableShortcuts` in task-table.tsx (Phase 1,
  // document-level). We intentionally do NOT register a window listener here:
  // doing so wipes the selection while the user is dismissing the inline tag
  // editor (which short-circuits the table handler via its `disabled` prop).

  // Defensive: page.tsx already guards the mount with
  // `{selection.selected.size > 0 && <BulkActionBar …>}`, so this branch is
  // unreachable in production. Retained so unit tests that render the bar
  // directly with an empty selection still see nothing.
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
      <button ref={tagButtonRef} type="button" onClick={onOpenTagEditor} className={primaryBtn}>
        Tag…
        <kbd className={kbdClass}>t</kbd>
      </button>

      {/* Secondary actions */}
      <button type="button" onClick={stageAll} className={secondaryBtn}>
        Stage
        <kbd className={kbdClass}>s</kbd>
      </button>
      <button type="button" onClick={markDone} className={secondaryBtn}>
        Mark done
        <kbd className={kbdClass}>d</kbd>
      </button>
      <button type="button" onClick={() => setConfirmCancel(true)} className={secondaryBtn}>
        Cancel
      </button>
      <button type="button" onClick={() => setConfirmDelete(true)} className={destructiveBtn}>
        Delete
      </button>

      {/* Meta + clear */}
      <span className={cn('ml-3 flex items-center gap-3 text-[12px] opacity-70')}>
        {canExpandMatching && (
          <button
            type="button"
            onClick={expandToMatching}
            className={cn(metaBtn, 'underline-offset-2')}
          >
            Select all matching · {totalMatching}
          </button>
        )}
        <span className="h-4 w-px bg-background/20" aria-hidden="true" />
        <button
          type="button"
          onClick={() => selection.clear()}
          className={cn(metaBtn, 'inline-flex items-center')}
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
