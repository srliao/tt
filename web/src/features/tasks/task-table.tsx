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
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  useDeleteTask,
  useReorderMain,
  useSetTaskState,
  useStageTask,
  useUnstageTask,
} from '@/api/tasks';
import type { Task, TaskSortAxis, TaskState } from '@/types/task';
import { TaskRow } from './task-row';

export interface TaskTableProps {
  tasks: Task[];
  sort: TaskSortAxis;
  selectedIds: Set<number>;
  onSelectedChange: (next: Set<number>) => void;
  onEdit: (task: Task) => void;
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
  hasFilters,
}: TaskTableProps) {
  const showDragHandle = sort === 'priority';
  const setState = useSetTaskState();
  const stage = useStageTask();
  const unstage = useUnstageTask();
  const del = useDeleteTask();
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

  const containerRef = useRef<HTMLTableElement>(null);
  const [focusedId, setFocusedId] = useState<number | null>(null);

  useTableShortcuts({
    containerRef,
    tasks: visible,
    focusedId,
    setFocusedId,
    selectedIds,
    onSelectedChange,
    onEdit,
    onSetState: (id, st) => setState.mutate({ id, state: st }),
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
          onSetState={(st) => setState.mutate({ id: task.id, state: st })}
          onStage={() => stage.mutate(task.id)}
          onUnstage={() => unstage.mutate(task.id)}
          onDelete={() => del.mutate(task.id)}
        />
      ))}
    </tbody>
  );

  return (
    <div className="flex flex-col gap-2">
      {showDragHandle && hasFilters && (
        <p className="text-xs text-muted-foreground">Filtered view — hidden tasks unchanged.</p>
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
                <th className="w-8 px-2 py-2 text-left font-medium" />
                {showDragHandle && <th className="w-6 px-1 py-2" />}
                <th className="px-2 py-2 text-left font-medium">Title</th>
                <th className="w-20 px-2 py-2 text-left font-medium">State</th>
                <th className="w-40 px-2 py-2 text-left font-medium">Tags</th>
                <th className="w-20 px-2 py-2 text-left font-medium">Due</th>
                <th className="w-16 px-2 py-2 text-left font-medium">Stage</th>
                <th className="w-8 px-1 py-2" />
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
  onSetState: (state: TaskState) => void;
  onStage: () => void;
  onUnstage: () => void;
  onDelete: () => void;
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
      onSetState={row.onSetState}
      onStage={row.onStage}
      onUnstage={row.onUnstage}
      onDelete={row.onDelete}
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
  onSetState: (id: number, state: TaskState) => void;
  onStage: (id: number) => void;
}

/**
 * j/k row navigation + enter/e/s/space/d action keys, scoped to the table
 * container. Only fires while the table itself or one of its descendants
 * is the focused element (avoids hijacking global typing).
 */
function useTableShortcuts({
  containerRef,
  tasks,
  focusedId,
  setFocusedId,
  selectedIds,
  onSelectedChange,
  onEdit,
  onSetState,
  onStage,
}: TableShortcutsArgs) {
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const handler = (event: KeyboardEvent) => {
      if (
        !containerRef.current?.contains(event.target as Node) &&
        event.target !== containerRef.current
      ) {
        return;
      }
      const target = event.target as HTMLElement | null;
      if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
        return;
      }
      if (tasks.length === 0) return;

      const currentIdx = focusedId != null ? tasks.findIndex((t) => t.id === focusedId) : -1;

      const setIdx = (idx: number) => {
        const clamped = Math.max(0, Math.min(tasks.length - 1, idx));
        setFocusedId(tasks[clamped].id);
      };

      if (event.key === 'j') {
        event.preventDefault();
        setIdx(currentIdx === -1 ? 0 : currentIdx + 1);
        return;
      }
      if (event.key === 'k') {
        event.preventDefault();
        setIdx(currentIdx === -1 ? 0 : currentIdx - 1);
        return;
      }
      if (focusedId == null) return;
      const focusedTask = tasks.find((t) => t.id === focusedId);
      if (!focusedTask) return;

      if (event.key === 'Enter' || event.key === 'e') {
        event.preventDefault();
        onEdit(focusedTask);
      } else if (event.key === 's') {
        event.preventDefault();
        onStage(focusedTask.id);
      } else if (event.key === 'd') {
        event.preventDefault();
        const next = focusedTask.state === 'done' ? 'not_done' : 'done';
        onSetState(focusedTask.id, next);
      } else if (event.key === ' ') {
        event.preventDefault();
        const copy = new Set(selectedIds);
        if (copy.has(focusedTask.id)) copy.delete(focusedTask.id);
        else copy.add(focusedTask.id);
        onSelectedChange(copy);
      }
    };

    el.addEventListener('keydown', handler);
    return () => el.removeEventListener('keydown', handler);
  }, [
    containerRef,
    tasks,
    focusedId,
    setFocusedId,
    selectedIds,
    onSelectedChange,
    onEdit,
    onSetState,
    onStage,
  ]);
}
