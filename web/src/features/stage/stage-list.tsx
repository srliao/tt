/**
 * Reorderable list of staged tasks. Wraps `StageRow` in a dnd-kit
 * `<DndContext>` / `<SortableContext>` (vertical strategy).
 *
 * Unlike the /tasks table this list isn't filtered, so the post-drop
 * neighbour calculation is straightforward: the row immediately above is
 * `before_id`, the row immediately below is `after_id`.
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
import { type CSSProperties, useEffect, useMemo, useState } from 'react';
import { useReorderStage } from '@/api/stage';
import { useSetTaskState, useUnstageTask } from '@/api/tasks';
import type { Task } from '@/types/task';
import { StageRow, toggleDone } from './stage-row';

export interface StageListProps {
  tasks: Task[];
  focusedId: number | null;
  onEdit: (task: Task) => void;
}

/** Pure helper exported for tests — runs the swap a dnd-kit drop would do. */
export function moveStaged(tasks: Task[], fromId: number, toId: number): Task[] {
  const fromIdx = tasks.findIndex((t) => t.id === fromId);
  const toIdx = tasks.findIndex((t) => t.id === toId);
  if (fromIdx === -1 || toIdx === -1 || fromIdx === toIdx) return tasks;
  const next = tasks.slice();
  const [moved] = next.splice(fromIdx, 1);
  next.splice(toIdx, 0, moved);
  return next;
}

/**
 * Compute the reorder payload from a post-drop list. The neighbours are the
 * row immediately above (`before_id`) and immediately below (`after_id`) the
 * moved row. Returns `null` neighbours for top/bottom positions.
 */
export function computeStagePayload(
  postDrop: Task[],
  movedId: number,
): { task_id: number; before_id: number | null; after_id: number | null } {
  const idx = postDrop.findIndex((t) => t.id === movedId);
  const before = idx > 0 ? postDrop[idx - 1].id : null;
  const after = idx < postDrop.length - 1 ? postDrop[idx + 1].id : null;
  return { task_id: movedId, before_id: before, after_id: after };
}

/**
 * Combined post-drop list + reorder payload — extracted so the dnd
 * `onDragEnd` handler stays trivial and tests can exercise the math without
 * dnd-kit.
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
  const next = moveStaged(visible, activeId, overId);
  if (next === visible) return null;
  return { next, payload: computeStagePayload(next, activeId) };
}

export function StageList({ tasks, focusedId, onEdit }: StageListProps) {
  const reorder = useReorderStage();
  const setState = useSetTaskState();
  const unstage = useUnstageTask();

  // Optimistic ordering. Resets whenever the underlying server list changes.
  const [optimistic, setOptimistic] = useState<Task[] | null>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: explicit reset on tasks change
  useEffect(() => {
    setOptimistic(null);
  }, [tasks]);
  const visible = optimistic ?? tasks;

  const ids = useMemo(() => visible.map((t) => t.id), [visible]);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const onDragEnd = (event: DragEndEvent) => {
    const activeId = Number(event.active.id);
    const overId = event.over ? Number(event.over.id) : null;
    const result = computeDragEnd(visible, activeId, overId);
    if (!result) return;
    setOptimistic(result.next);
    reorder.mutate(result.payload);
  };

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <SortableContext items={ids} strategy={verticalListSortingStrategy}>
        <div data-stage-list className="flex flex-col gap-1.5">
          {visible.map((task) => (
            <SortableStageRow
              key={task.id}
              task={task}
              focused={task.id === focusedId}
              onEdit={() => onEdit(task)}
              onToggleDone={() => setState.mutate({ id: task.id, state: toggleDone(task.state) })}
              onUnstage={() => unstage.mutate(task.id)}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}

interface SortableStageRowProps {
  task: Task;
  focused: boolean;
  onEdit: () => void;
  onToggleDone: () => void;
  onUnstage: () => void;
}

function SortableStageRow({
  task,
  focused,
  onEdit,
  onToggleDone,
  onUnstage,
}: SortableStageRowProps) {
  const { setNodeRef, transform, transition, attributes, listeners, isDragging } = useSortable({
    id: task.id,
  });

  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <StageRow
      ref={setNodeRef}
      style={style}
      task={task}
      focused={focused}
      dragHandle={
        <button
          type="button"
          {...attributes}
          {...listeners}
          aria-label={`Reorder ${task.title}`}
          className="inline-flex size-6 cursor-grab items-center justify-center text-muted-foreground hover:text-foreground active:cursor-grabbing"
        >
          <GripVerticalIcon className="size-4" />
        </button>
      }
      onEdit={onEdit}
      onToggleDone={onToggleDone}
      onUnstage={onUnstage}
    />
  );
}
