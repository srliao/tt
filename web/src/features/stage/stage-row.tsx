/**
 * Single staged task row rendered inside `<StageList>`. Flexbox row so dnd-kit
 * can hoist a single sortable item without table-cell shimming.
 *
 * Layout (left → right): drag handle, done radio, content (title + tags +
 * due), bookmark (toggles stage/unstage). Done/cancelled rows are visually
 * de-emphasised but stay in place at their current `staged_order`.
 */

import { format, isPast, parseISO } from 'date-fns';
import { AlertTriangleIcon, BookmarkIcon, CheckIcon, GripVerticalIcon } from 'lucide-react';
import { type CSSProperties, forwardRef, type ReactNode } from 'react';
import { Badge } from '@/components/ui/badge';
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
  onToggleDone: () => void;
  onUnstage: () => void;
}

/**
 * Toggle helper: done ↔ not_done. Cancelled tasks become done on click (the
 * radio shows them as "not done"). Cancelling is no longer a click action;
 * it lives in the edit modal.
 */
export function toggleDone(state: TaskState): TaskState {
  return state === 'done' ? 'not_done' : 'done';
}

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

/**
 * Done-state radio. Empty circle for not_done/cancelled; filled with check
 * for done. Cancelled rows still get a strikethrough on the title.
 */
function DoneRadio({
  state,
  onClick,
  title,
}: {
  state: TaskState;
  onClick: () => void;
  title: string;
}) {
  const done = state === 'done';
  return (
    <button
      type="button"
      aria-pressed={done}
      aria-label={`Mark ${title} as ${done ? 'not done' : 'done'}`}
      onClick={onClick}
      className={cn(
        'inline-flex size-5 shrink-0 items-center justify-center rounded-full border transition-colors',
        done
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-muted-foreground/60 text-transparent hover:border-foreground',
      )}
    >
      <CheckIcon className="size-3" aria-hidden="true" />
    </button>
  );
}

export const StageRow = forwardRef<HTMLDivElement, StageRowProps>(function StageRow(
  { task, focused, dragHandle, style, onEdit, onToggleDone, onUnstage },
  ref,
) {
  const finished = task.state === 'done' || task.state === 'cancelled';

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

      <DoneRadio state={task.state} onClick={onToggleDone} title={task.title} />

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

      <button
        type="button"
        aria-label={`Unstage ${task.title}`}
        aria-pressed={true}
        onClick={onUnstage}
        className="inline-flex size-7 shrink-0 items-center justify-center rounded-md text-primary transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        <BookmarkIcon className="size-4 fill-current" aria-hidden="true" />
      </button>
    </div>
  );
});
