/**
 * Single task row rendered inside `<TaskTable>`. Kept in its own file so the
 * table can stay focused on layout/keyboard handling.
 *
 * Column order (left → right): multi-select checkbox (always rendered, faded
 * via group-hover/row until the row is selected or hovered), optional drag
 * handle (only when sort=priority), done radio, title (+ notes), tags, due,
 * bookmark on the right that toggles stage/unstage. Edit, delete, and "mark
 * cancelled" live in the edit modal — there's no kebab menu.
 */

import { format, isPast, parseISO } from 'date-fns';
import { AlertTriangleIcon, BookmarkIcon, CheckIcon, GripVerticalIcon } from 'lucide-react';
import { type CSSProperties, forwardRef, type MouseEvent, type ReactNode } from 'react';
import { Checkbox } from '@/components/ui/checkbox';
import { TagGlyphList } from '@/components/ui/tag-glyph';
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
  onToggleDone: () => void;
  onStage: () => void;
  onUnstage: () => void;
  /**
   * Map of tag-name → 1- or 2-letter glyph initial, computed once per table
   * render off the visible tag set. Passed down so every row gets the same
   * disambiguation without recomputing.
   */
  initialMap: Map<string, string>;
  /** Click handler for a single tag glyph — usually filters by that tag. */
  onTagClick: (name: string, event: MouseEvent) => void;
}

/** Toggle helper: done ↔ not_done. Cancelled becomes done on click. */
export function toggleDoneState(state: TaskState): TaskState {
  return state === 'done' ? 'not_done' : 'done';
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
    onToggleDone,
    onStage,
    onUnstage,
    initialMap,
    onTagClick,
  },
  ref,
) {
  const staged = task.staged_order !== null;
  const finished = task.state === 'done' || task.state === 'cancelled';

  return (
    <tr
      ref={ref}
      style={style}
      data-task-id={task.id}
      data-focused={focused || undefined}
      data-selected={selected || undefined}
      data-state={task.state}
      className={cn(
        'group/row relative border-b hover:bg-muted/40',
        // Focus state — distinct from hover. The leading-edge rail is a
        // pseudo-element pinned to the row's left edge; the subtle outline
        // helps the bar stand out against dark backgrounds.
        'data-focused:bg-primary/12',
        'data-focused:before:absolute data-focused:before:left-0 data-focused:before:top-1 data-focused:before:bottom-1 data-focused:before:w-[3px] data-focused:before:bg-primary data-focused:before:rounded-r-sm',
        'data-focused:outline data-focused:outline-1 data-focused:-outline-offset-1 data-focused:outline-primary/25',
        selected && 'bg-primary/8',
        finished && 'opacity-60',
      )}
    >
      <td className="px-2 py-2 align-middle">
        <Checkbox
          checked={selected}
          onCheckedChange={(c) => onToggleSelect(c === true)}
          aria-label={`Select ${task.title}`}
          className={cn(
            'opacity-0 transition-opacity duration-100',
            'group-hover/row:opacity-60 data-[state=checked]:opacity-100',
            selected && 'opacity-100',
          )}
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
        <DoneRadio state={task.state} onClick={onToggleDone} title={task.title} />
      </td>
      <td className="px-2 py-2 align-middle">
        <button
          type="button"
          className={cn(
            'text-left text-sm font-medium hover:underline',
            finished && 'line-through text-muted-foreground',
          )}
          onClick={onEdit}
        >
          {task.title}
        </button>
        {task.notes && <p className="line-clamp-1 text-xs text-muted-foreground">{task.notes}</p>}
      </td>
      <td className="px-2 py-2 align-middle">
        <TagGlyphList tags={task.tags} initialMap={initialMap} onTagClick={onTagClick} />
      </td>
      <td className="px-2 py-2 align-middle">{dueBadge(task)}</td>
      <td className="px-2 py-2 align-middle text-right">
        <button
          type="button"
          onClick={() => (staged ? onUnstage() : onStage())}
          aria-label={staged ? `Unstage ${task.title}` : `Stage ${task.title}`}
          aria-pressed={staged}
          data-staged={staged || undefined}
          className={cn(
            'inline-flex size-7 items-center justify-center rounded-md transition-colors',
            'hover:bg-accent hover:text-accent-foreground',
            staged ? 'text-primary' : 'text-muted-foreground',
          )}
        >
          <BookmarkIcon className={cn('size-4', staged && 'fill-current')} aria-hidden="true" />
        </button>
      </td>
    </tr>
  );
});
