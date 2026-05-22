/**
 * Renders the list of tasks as a table. Owns:
 *
 * - column headers + per-row dispatch into TaskRow
 * - keyboard navigation (j/k/enter/e/s/space/d) via `useTableShortcuts`
 * - drag-drop reorder when `sort === 'priority'` (Task 5)
 */

import {
  closestCenter,
  DndContext,
  type DragEndEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVerticalIcon } from 'lucide-react';
import { type MouseEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useReorderMain, useSetTaskState, useStageTask, useUnstageTask } from '@/api/tasks';
import { buildInitialMap } from '@/lib/tag-initials';
import type { Task, TaskSortAxis } from '@/types/task';
import { TaskRow, toggleDoneState } from './task-row';
import { clickModeFromEvent, useTagFilterMutator } from './use-task-list-search';

export interface TaskTableProps {
  tasks: Task[];
  sort: TaskSortAxis;
  selectedIds: Set<number>;
  onSelectedChange: (next: Set<number>) => void;
  onEdit: (task: Task) => void;
  /** Opens the inline tag editor over the focused task's tag cell (`t`). */
  onEditTags?: (task: Task) => void;
  /**
   * When set, the keyboard shortcuts are inert. The inline tag editor flips
   * this on so its own input can own keystrokes without the table eating
   * `t`, `d`, etc.
   */
  shortcutsDisabled?: boolean;
  /**
   * Whether any filter is active. When true and DnD is on, show a hint that
   * the reorder operates only on the visible subset.
   */
  hasFilters?: boolean;
}

/**
 * Exported for tests — moves the task with `fromId` to the position currently
 * occupied by `toId`. Returns the same array reference when the move would
 * have no effect.
 */
export function moveTask(tasks: Task[], fromId: number, toId: number): Task[] {
  const fromIdx = tasks.findIndex((t) => t.id === fromId);
  const toIdx = tasks.findIndex((t) => t.id === toId);
  if (fromIdx === -1 || toIdx === -1 || fromIdx === toIdx) return tasks;
  const next = tasks.slice();
  const [moved] = next.splice(fromIdx, 1);
  next.splice(toIdx, 0, moved);
  return next;
}

/**
 * Compute the dnd-kit reorder payload for a list-after-drop. Exported so the
 * test can exercise the neighbour calculation directly.
 */
export function computeReorderPayload(
  postDrop: Task[],
  movedId: number,
): { task_id: number; before_id: number | null; after_id: number | null } {
  const idx = postDrop.findIndex((t) => t.id === movedId);
  const before = idx > 0 ? postDrop[idx - 1].id : null;
  const after = idx < postDrop.length - 1 ? postDrop[idx + 1].id : null;
  return { task_id: movedId, before_id: before, after_id: after };
}

/**
 * Combined post-drop list + reorder payload — extracted so the dnd `onDragEnd`
 * handler stays trivial and tests can exercise the math without dnd-kit.
 */
export function computeDragEnd(
  visible: Task[],
  activeId: number,
  overId: number | null,
): {
  next: Task[];
  payload: { task_id: number; before_id: number | null; after_id: number | null };
} | null {
  if (!overId || activeId === overId) return null;
  const next = moveTask(visible, activeId, overId);
  if (next === visible) return null;
  return { next, payload: computeReorderPayload(next, activeId) };
}

export function TaskTable({
  tasks,
  sort,
  selectedIds,
  onSelectedChange,
  onEdit,
  onEditTags,
  shortcutsDisabled,
  hasFilters,
}: TaskTableProps) {
  const showDragHandle = sort === 'priority';
  const setState = useSetTaskState();
  const stage = useStageTask();
  const unstage = useUnstageTask();
  const reorder = useReorderMain();

  // Optimistic ordering for drag-drop. Resets whenever the underlying server
  // list changes (e.g. invalidation completed). The dep on `tasks` is the
  // *whole point* — we want to clear the override when the server response
  // updates — so the lint warning is suppressed.
  const [optimistic, setOptimistic] = useState<Task[] | null>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: explicit reset on tasks change
  useEffect(() => {
    setOptimistic(null);
  }, [tasks]);
  const visible = optimistic ?? tasks;

  const ids = useMemo(() => visible.map((t) => t.id), [visible]);

  // Compute initials once per render off the union of visible tags. Doing it
  // here (vs. per-row or off the global tag set) keeps glyphs short when
  // most of the user's tags are inactive — see Phase 4 spec.
  const initialMap = useMemo(() => {
    const set = new Set<string>();
    for (const t of visible) for (const tag of t.tags) set.add(tag);
    return buildInitialMap([...set]);
  }, [visible]);

  const mutateTagFilter = useTagFilterMutator();
  const onTagClick = useCallback(
    (name: string, event: MouseEvent) => {
      event.stopPropagation();
      mutateTagFilter(name, clickModeFromEvent(event));
    },
    [mutateTagFilter],
  );

  const containerRef = useRef<HTMLTableElement>(null);
  const [focusedId, setFocusedId] = useState<number | null>(null);

  // Keep the focused row in view as j/k walks past the viewport. Uses
  // window.scrollBy (not Element.scrollIntoView, per repo convention) and
  // an instant behavior — smooth scrolling on every step feels laggy when
  // holding the key down.
  useEffect(() => {
    if (focusedId == null) return;
    const row = document.querySelector<HTMLElement>(`[data-task-id="${focusedId}"]`);
    if (!row) return;
    const rect = row.getBoundingClientRect();
    const topPad = 80; // top nav + active filter strip
    const botPad = 80; // bulk action bar margin
    if (rect.top < topPad) {
      window.scrollBy({ top: rect.top - topPad, behavior: 'instant' });
    } else if (rect.bottom > window.innerHeight - botPad) {
      window.scrollBy({
        top: rect.bottom - (window.innerHeight - botPad),
        behavior: 'instant',
      });
    }
  }, [focusedId]);

  useTableShortcuts({
    containerRef,
    tasks: visible,
    focusedId,
    setFocusedId,
    selectedIds,
    onSelectedChange,
    onEdit,
    onEditTags,
    disabled: shortcutsDisabled,
    onToggleDone: (id, st) => setState.mutate({ id, state: st }),
    onStage: (id) => stage.mutate(id),
  });

  const toggleSelect = (taskId: number, next: boolean) => {
    const copy = new Set(selectedIds);
    if (next) copy.add(taskId);
    else copy.delete(taskId);
    onSelectedChange(copy);
  };

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const onDragEnd = (event: DragEndEvent) => {
    const activeId = Number(event.active.id);
    const overId = event.over ? Number(event.over.id) : null;
    const result = computeDragEnd(visible, activeId, overId);
    if (!result) return;
    setOptimistic(result.next);
    reorder.mutate(result.payload);
  };

  const tableBody = (
    <tbody>
      {visible.map((task) => (
        <SortableRow
          key={task.id}
          enabled={showDragHandle}
          task={task}
          focused={task.id === focusedId}
          selected={selectedIds.has(task.id)}
          onToggleSelect={(next) => toggleSelect(task.id, next)}
          showDragHandle={showDragHandle}
          onEdit={() => onEdit(task)}
          onToggleDone={() => setState.mutate({ id: task.id, state: toggleDoneState(task.state) })}
          onStage={() => stage.mutate(task.id)}
          onUnstage={() => unstage.mutate(task.id)}
          initialMap={initialMap}
          onTagClick={onTagClick}
        />
      ))}
    </tbody>
  );

  return (
    <div className="flex flex-col gap-2">
      {showDragHandle && hasFilters && (
        <p className="text-xs text-muted-foreground">Filtered view — hidden tasks unchanged.</p>
      )}
      {focusedId == null && visible.length > 0 && (
        <div
          className="ml-auto -mb-2 inline-flex items-center gap-1.5 self-end rounded-full border border-dashed border-primary/30 bg-primary/5 px-2.5 py-0.5 text-[11px] font-mono text-primary"
          aria-hidden="true"
        >
          Press <kbd className="font-mono text-[10px]">j</kbd> to navigate
        </div>
      )}
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext
          items={ids}
          strategy={verticalListSortingStrategy}
          disabled={!showDragHandle}
        >
          <table
            ref={containerRef}
            aria-label="Tasks"
            className="w-full table-fixed text-sm focus-visible:outline-none"
            data-task-table
          >
            <thead className="text-xs text-muted-foreground">
              <tr className="border-b">
                <th className="w-8 px-2 py-2" />
                {showDragHandle && <th className="w-6 px-1 py-2" />}
                <th className="w-8 px-2 py-2" />
                <th className="px-2 py-2 text-left font-medium">Title</th>
                <th className="w-24 px-2 py-2 text-left font-medium">Tags</th>
                <th className="w-20 px-2 py-2 text-left font-medium">Due</th>
                <th className="w-10 px-2 py-2" />
              </tr>
            </thead>
            {tableBody}
          </table>
        </SortableContext>
      </DndContext>
    </div>
  );
}

interface SortableRowProps {
  enabled: boolean;
  task: Task;
  focused: boolean;
  selected: boolean;
  onToggleSelect: (next: boolean) => void;
  showDragHandle: boolean;
  onEdit: () => void;
  onToggleDone: () => void;
  onStage: () => void;
  onUnstage: () => void;
  initialMap: Map<string, string>;
  onTagClick: (name: string, event: MouseEvent) => void;
}

/**
 * Bridges dnd-kit's `useSortable` into a TaskRow. The hook is always called
 * (rules-of-hooks) but only wires up listeners when `enabled` is true.
 */
function SortableRow({ enabled, ...row }: SortableRowProps) {
  const { setNodeRef, transform, transition, attributes, listeners, isDragging } = useSortable({
    id: row.task.id,
    disabled: !enabled,
  });

  const style: React.CSSProperties = enabled
    ? {
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
      }
    : {};

  return (
    <TaskRow
      ref={setNodeRef}
      style={style}
      task={row.task}
      focused={row.focused}
      selected={row.selected}
      onToggleSelect={row.onToggleSelect}
      showDragHandle={row.showDragHandle}
      dragHandle={
        enabled ? (
          <button
            type="button"
            {...attributes}
            {...listeners}
            aria-label={`Reorder ${row.task.title}`}
            className="inline-flex size-6 cursor-grab items-center justify-center text-muted-foreground hover:text-foreground active:cursor-grabbing"
          >
            <GripVerticalIcon className="size-4" />
          </button>
        ) : null
      }
      onEdit={row.onEdit}
      onToggleDone={row.onToggleDone}
      onStage={row.onStage}
      onUnstage={row.onUnstage}
      initialMap={row.initialMap}
      onTagClick={row.onTagClick}
    />
  );
}

interface TableShortcutsArgs {
  containerRef: React.RefObject<HTMLTableElement | null>;
  tasks: Task[];
  focusedId: number | null;
  setFocusedId: (id: number | null) => void;
  selectedIds: Set<number>;
  onSelectedChange: (next: Set<number>) => void;
  onEdit: (task: Task) => void;
  onEditTags?: (task: Task) => void;
  /** When true, swallow no keys. Used while the inline tag editor is open. */
  disabled?: boolean;
  onToggleDone: (id: number, state: ReturnType<typeof toggleDoneState>) => void;
  onStage: (id: number) => void;
}

/**
 * Compute the new selection set from an anchor + cursor pair. Exported so
 * the unit test can exercise the math without driving a full table render.
 *
 * Semantics: every task whose index lies between `anchorIdx` and
 * `cursorIdx` (inclusive) is selected; nothing else is selected. This is
 * the "fresh range" model — the table doesn't try to preserve unrelated
 * checkboxes the user toggled before starting a range, because the
 * shift-walk should be predictable.
 */
export function rangeSelection(tasks: Task[], anchorIdx: number, cursorIdx: number): Set<number> {
  if (anchorIdx < 0 || cursorIdx < 0) return new Set();
  const lo = Math.min(anchorIdx, cursorIdx);
  const hi = Math.max(anchorIdx, cursorIdx);
  const out = new Set<number>();
  for (let i = lo; i <= hi; i++) {
    if (tasks[i]) out.add(tasks[i].id);
  }
  return out;
}

/**
 * j/k row navigation + enter/e/s/space/d action keys, scoped to the table
 * container. Only fires while the table itself or one of its descendants
 * is the focused element (avoids hijacking global typing).
 *
 * ⇧j/⇧k start (or extend) a range select rooted at the focused row when
 * the shift modifier is first pressed. The anchor resets whenever the user
 * does anything that breaks the range — clicks a checkbox, presses an
 * unrelated key, or steps with plain j/k.
 */
function useTableShortcuts({
  containerRef,
  tasks,
  focusedId,
  setFocusedId,
  selectedIds,
  onSelectedChange,
  onEdit,
  onEditTags,
  disabled,
  onToggleDone,
  onStage,
}: TableShortcutsArgs) {
  const anchorRef = useRef<number | null>(null);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (disabled) return;
      // Don't hijack keys when the user is typing.
      const target = event.target as HTMLElement | null;
      if (
        target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement ||
        target?.isContentEditable
      ) {
        return;
      }
      // Defer to any open Radix dialog/popover — they own keys while visible.
      if (
        document.querySelector(
          '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"]',
        )
      ) {
        return;
      }
      if (tasks.length === 0) return;

      const currentIdx = focusedId != null ? tasks.findIndex((t) => t.id === focusedId) : -1;

      const setIdx = (idx: number) => {
        const clamped = Math.max(0, Math.min(tasks.length - 1, idx));
        setFocusedId(tasks[clamped].id);
      };

      // Escape clears the selection. We only consume the key when there's
      // something to clear, so other Escape consumers (e.g. closing a
      // popover) keep working when the table is in its default state.
      // Inputs/textareas were already filtered above; the `disabled` guard
      // handles the modal-open case.
      if (event.key === 'Escape') {
        if (selectedIds.size > 0) {
          event.preventDefault();
          event.stopPropagation();
          anchorRef.current = null;
          onSelectedChange(new Set());
        }
        return;
      }

      // ⇧ + letter delivers a capital letter. We branch on those so the
      // logic stays clear: lower-case = plain step (resets anchor),
      // upper-case = extend range (anchor sticky).
      if (event.key === 'J' || event.key === 'K') {
        event.preventDefault();
        if (currentIdx === -1) {
          // Nothing focused yet — first ⇧j just focuses row 0; no range.
          setIdx(0);
          anchorRef.current = tasks[0].id;
          return;
        }
        if (anchorRef.current == null) anchorRef.current = focusedId;
        const nextIdx = Math.max(
          0,
          Math.min(tasks.length - 1, event.key === 'J' ? currentIdx + 1 : currentIdx - 1),
        );
        setFocusedId(tasks[nextIdx].id);
        const anchorIdx = tasks.findIndex((t) => t.id === anchorRef.current);
        onSelectedChange(rangeSelection(tasks, anchorIdx, nextIdx));
        return;
      }

      if (event.key === 'j') {
        event.preventDefault();
        anchorRef.current = null;
        setIdx(currentIdx === -1 ? 0 : currentIdx + 1);
        return;
      }
      if (event.key === 'k') {
        event.preventDefault();
        anchorRef.current = null;
        setIdx(currentIdx === -1 ? 0 : currentIdx - 1);
        return;
      }
      if (focusedId == null) {
        anchorRef.current = null;
        return;
      }
      const focusedTask = tasks.find((t) => t.id === focusedId);
      if (!focusedTask) {
        anchorRef.current = null;
        return;
      }

      if (event.key === 'Enter' || event.key === 'e') {
        event.preventDefault();
        anchorRef.current = null;
        onEdit(focusedTask);
      } else if (event.key === 's') {
        event.preventDefault();
        anchorRef.current = null;
        onStage(focusedTask.id);
      } else if (event.key === 'd') {
        event.preventDefault();
        anchorRef.current = null;
        onToggleDone(focusedTask.id, toggleDoneState(focusedTask.state));
      } else if (event.key === 't') {
        if (!onEditTags) return;
        event.preventDefault();
        anchorRef.current = null;
        onEditTags(focusedTask);
      } else if (event.key === ' ' || event.key === 'x') {
        event.preventDefault();
        anchorRef.current = null;
        const copy = new Set(selectedIds);
        if (copy.has(focusedTask.id)) copy.delete(focusedTask.id);
        else copy.add(focusedTask.id);
        onSelectedChange(copy);
      } else {
        // Any other letter / modifier-free key — break the range.
        anchorRef.current = null;
      }
    };

    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [
    tasks,
    focusedId,
    setFocusedId,
    selectedIds,
    onSelectedChange,
    onEdit,
    onEditTags,
    onToggleDone,
    onStage,
    disabled,
  ]);

  // A click on a row checkbox resets the range anchor — Phase 6 spec.
  // The table renders the checkbox column inside `containerRef`, so a
  // delegated listener here keeps the policy in one place.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onClick = (event: MouseEvent) => {
      const t = event.target as HTMLElement | null;
      if (t?.closest('[role="checkbox"], input[type="checkbox"]')) {
        anchorRef.current = null;
      }
    };
    el.addEventListener('click', onClick);
    return () => el.removeEventListener('click', onClick);
  }, [containerRef]);
}
