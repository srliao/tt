/**
 * Single task row rendered inside `<TaskTable>`. Kept in its own file so the
 * table can stay focused on layout/keyboard handling while individual rows
 * own click + kebab-menu behaviour.
 *
 * Renders columns in spec §6 order: bulk-select checkbox, drag handle
 * (conditional), title (click → edit), state pill, tag chips, due date
 * (overdue indicator), staged badge, kebab menu.
 */

import { format, isPast, parseISO } from 'date-fns';
import { AlertTriangleIcon, GripVerticalIcon, MoreHorizontalIcon } from 'lucide-react';
import { type CSSProperties, forwardRef, type ReactNode } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import type { Task, TaskState } from '@/types/task';

export interface TaskRowProps {
  task: Task;
  /** When true, the row is currently keyboard-focused (j/k navigation). */
  focused?: boolean;
  /** When true, the row is bulk-selected (checkbox checked). */
  selected: boolean;
  onToggleSelect: (next: boolean) => void;
  /** Whether the drag-handle column should be rendered. */
  showDragHandle: boolean;
  /** Optional slot — the dnd-kit hook injects the listeners/handle here. */
  dragHandle?: ReactNode;
  /** Inline style — used to apply dnd-kit transforms on the row. */
  style?: CSSProperties;
  onEdit: () => void;
  onSetState: (state: TaskState) => void;
  onStage: () => void;
  onUnstage: () => void;
  onDelete: () => void;
}

function dueBadge(task: Task) {
  if (!task.due_date) return null;
  let dt: Date;
  try {
    // due_date is YYYY-MM-DD; parseISO treats that as midnight UTC. For the
    // "is it past?" comparison we want local-midnight semantics, but the
    // small drift is acceptable for this UI hint.
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

function stateVariant(state: TaskState): 'default' | 'secondary' | 'outline' {
  switch (state) {
    case 'done':
      return 'secondary';
    case 'cancelled':
      return 'outline';
    default:
      return 'default';
  }
}

const STATE_LABEL: Record<TaskState, string> = {
  not_done: 'Open',
  done: 'Done',
  cancelled: 'Cancelled',
};

export const TaskRow = forwardRef<HTMLTableRowElement, TaskRowProps>(function TaskRow(
  {
    task,
    focused,
    selected,
    onToggleSelect,
    showDragHandle,
    dragHandle,
    style,
    onEdit,
    onSetState,
    onStage,
    onUnstage,
    onDelete,
  },
  ref,
) {
  return (
    <tr
      ref={ref}
      style={style}
      data-task-id={task.id}
      data-focused={focused || undefined}
      data-selected={selected || undefined}
      className="border-b hover:bg-muted/40 data-[focused]:bg-accent/40"
    >
      <td className="px-2 py-2 align-middle">
        <Checkbox
          checked={selected}
          onCheckedChange={(c) => onToggleSelect(c === true)}
          aria-label={`Select ${task.title}`}
        />
      </td>
      {showDragHandle && (
        <td className="px-1 py-2 align-middle">
          {dragHandle ?? (
            <span className="inline-flex size-6 items-center justify-center text-muted-foreground">
              <GripVerticalIcon className="size-4" aria-hidden="true" />
            </span>
          )}
        </td>
      )}
      <td className="px-2 py-2 align-middle">
        <button
          type="button"
          className="text-left text-sm font-medium hover:underline"
          onClick={onEdit}
        >
          {task.title}
        </button>
        {task.notes && <p className="line-clamp-1 text-xs text-muted-foreground">{task.notes}</p>}
      </td>
      <td className="px-2 py-2 align-middle">
        <Badge variant={stateVariant(task.state)} className="text-[10px]">
          {STATE_LABEL[task.state]}
        </Badge>
      </td>
      <td className="px-2 py-2 align-middle">
        <div className="flex flex-wrap gap-1">
          {task.tags.map((tag) => (
            <Badge key={tag} variant="outline" className="text-[10px]">
              {tag}
            </Badge>
          ))}
        </div>
      </td>
      <td className="px-2 py-2 align-middle">{dueBadge(task)}</td>
      <td className="px-2 py-2 align-middle">
        {task.staged_order !== null && (
          <Badge variant="secondary" className="text-[10px]">
            ·staged
          </Badge>
        )}
      </td>
      <td className="px-1 py-2 align-middle">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${task.title}`}>
              <MoreHorizontalIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onEdit}>Edit</DropdownMenuItem>
            <DropdownMenuSeparator />
            {task.state !== 'done' && (
              <DropdownMenuItem onClick={() => onSetState('done')}>Mark done</DropdownMenuItem>
            )}
            {task.state !== 'not_done' && (
              <DropdownMenuItem onClick={() => onSetState('not_done')}>
                Mark not done
              </DropdownMenuItem>
            )}
            {task.state !== 'cancelled' && (
              <DropdownMenuItem onClick={() => onSetState('cancelled')}>
                Mark cancelled
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            {task.staged_order === null ? (
              <DropdownMenuItem onClick={onStage}>Stage</DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={onUnstage}>Unstage</DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={onDelete}>
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </td>
    </tr>
  );
});
