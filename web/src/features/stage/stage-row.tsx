/**
 * Single staged task row rendered inside `<StageList>`. Unlike the /tasks
 * table this is a flexbox row (not a `<tr>`) so dnd-kit can hoist a single
 * sortable item without table-cell shimming.
 *
 * Done/cancelled rows are visually de-emphasised (strikethrough title +
 * desaturated background) but stay in place at their current `staged_order`,
 * per spec §6.
 */

import { format, isPast, parseISO } from 'date-fns';
import {
  AlertTriangleIcon,
  CheckCircle2Icon,
  CircleDashedIcon,
  CircleIcon,
  GripVerticalIcon,
  XCircleIcon,
  XIcon,
} from 'lucide-react';
import { type CSSProperties, forwardRef, type ReactNode } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { Task, TaskState } from '@/types/task';

export interface StageRowProps {
  task: Task;
  /** When true, this row is currently keyboard-focused (j/k navigation). */
  focused?: boolean;
  /** Optional dnd-kit drag handle injected by `<StageList>`. */
  dragHandle?: ReactNode;
  /** Inline style — used to apply dnd-kit transforms on the row. */
  style?: CSSProperties;
  onEdit: () => void;
  onCycleState: () => void;
  onUnstage: () => void;
}

/** State-cycle order used by the inline toggle button + `d` shortcut. */
export function nextState(state: TaskState): TaskState {
  switch (state) {
    case 'not_done':
      return 'done';
    case 'done':
      return 'cancelled';
    case 'cancelled':
      return 'not_done';
  }
}

const STATE_ICON: Record<TaskState, typeof CircleIcon> = {
  not_done: CircleIcon,
  done: CheckCircle2Icon,
  cancelled: CircleDashedIcon,
};

const STATE_LABEL: Record<TaskState, string> = {
  not_done: 'Not done',
  done: 'Done',
  cancelled: 'Cancelled',
};

function dueBadge(task: Task) {
  if (!task.due_date) return null;
  let dt: Date;
  try {
    dt = parseISO(task.due_date);
  } catch {
    return null;
  }
  const overdue = isPast(dt) && task.state === 'not_done';
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 text-xs',
        overdue ? 'text-destructive' : 'text-muted-foreground',
      )}
    >
      {overdue && <AlertTriangleIcon className="size-3" aria-hidden="true" />}
      {format(dt, 'MMM d')}
    </span>
  );
}

export const StageRow = forwardRef<HTMLDivElement, StageRowProps>(function StageRow(
  { task, focused, dragHandle, style, onEdit, onCycleState, onUnstage },
  ref,
) {
  const finished = task.state === 'done' || task.state === 'cancelled';
  const StateIcon = STATE_ICON[task.state];

  return (
    <div
      ref={ref}
      style={style}
      data-task-id={task.id}
      data-focused={focused || undefined}
      data-state={task.state}
      className={cn(
        'flex items-center gap-2 rounded-md border bg-card px-2 py-2 text-sm',
        'hover:bg-muted/40 data-[focused]:bg-accent/40',
        finished && 'opacity-60 bg-muted/30',
      )}
    >
      {dragHandle ?? (
        <span className="inline-flex size-6 items-center justify-center text-muted-foreground">
          <GripVerticalIcon className="size-4" aria-hidden="true" />
        </span>
      )}

      <button
        type="button"
        aria-label={`Cycle state for ${task.title} (currently ${STATE_LABEL[task.state]})`}
        className={cn(
          'inline-flex size-6 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent',
          task.state === 'done' && 'text-emerald-600 dark:text-emerald-400',
          task.state === 'cancelled' && 'text-destructive',
        )}
        onClick={onCycleState}
      >
        <StateIcon className="size-4" aria-hidden="true" />
      </button>

      <button
        type="button"
        className={cn(
          'flex-1 truncate text-left font-medium hover:underline',
          finished && 'line-through text-muted-foreground',
        )}
        onClick={onEdit}
      >
        {task.title}
      </button>

      <div className="flex shrink-0 flex-wrap gap-1">
        {task.tags.map((tag) => (
          <Badge key={tag} variant="outline" className="text-[10px]">
            {tag}
          </Badge>
        ))}
      </div>

      <div className="shrink-0">{dueBadge(task)}</div>

      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={`Unstage ${task.title}`}
        onClick={onUnstage}
      >
        {task.state === 'cancelled' ? (
          <XCircleIcon className="size-4" aria-hidden="true" />
        ) : (
          <XIcon className="size-4" aria-hidden="true" />
        )}
      </Button>
    </div>
  );
});
