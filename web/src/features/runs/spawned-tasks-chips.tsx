/**
 * Renders one chip per task that a run spawned. The detail endpoint already
 * inlines the matched task summaries (id/title/state), so this component is a
 * pure presentation layer — no extra fetches required.
 *
 * Deleted tasks: the server omits any id that no longer resolves, so any id
 * in `spawnedIds` but missing from `tasks` is rendered as a "deleted" chip
 * with strikethrough text. Cancelled tasks (state='cancelled') also get the
 * strikethrough treatment so the visual signal matches.
 *
 * Chips are buttons / links — clicking jumps to /tasks. The list page reads
 * a `?highlight` search param when present (future enhancement); for v1 we
 * pass it through anyway so the chip remains a useful navigation affordance.
 */

import { Link } from '@tanstack/react-router';
import type { SpawnedTaskSummary } from '@/api/runs';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import type { TaskState } from '@/types/task';

export interface SpawnedTasksChipsProps {
  /** All ids the run reports as spawned. Used to detect deletions. */
  spawnedIds: number[];
  /** Summaries the server returned for the still-existing tasks. */
  tasks: SpawnedTaskSummary[];
}

const STATE_LABEL: Record<TaskState, string> = {
  not_done: 'open',
  done: 'done',
  cancelled: 'cancelled',
};

const STATE_CLASSES: Record<TaskState, string> = {
  not_done: '',
  done: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-900/40 dark:text-emerald-200',
  cancelled: 'text-muted-foreground',
};

export function SpawnedTasksChips({ spawnedIds, tasks }: SpawnedTasksChipsProps) {
  const byId = new Map(tasks.map((t) => [t.id, t]));

  return (
    <ul className="flex flex-wrap gap-2">
      {spawnedIds.map((id) => {
        const task = byId.get(id);
        if (!task) {
          return (
            <li key={id}>
              <DeletedChip id={id} />
            </li>
          );
        }
        return (
          <li key={id}>
            <TaskChip task={task} />
          </li>
        );
      })}
    </ul>
  );
}

function TaskChip({ task }: { task: SpawnedTaskSummary }) {
  const struck = task.state === 'cancelled';
  return (
    <Link
      to="/tasks"
      className={cn(
        'group inline-flex items-center gap-2 rounded-full border bg-card px-3 py-1 text-xs hover:bg-muted/50',
        struck && 'opacity-70',
      )}
      data-task-id={task.id}
      title={`Task #${task.id}: ${task.title}`}
    >
      <span className={cn('truncate max-w-[18rem]', struck && 'line-through')}>{task.title}</span>
      <Badge variant="outline" className={cn('text-[10px]', STATE_CLASSES[task.state])}>
        {STATE_LABEL[task.state]}
      </Badge>
    </Link>
  );
}

function DeletedChip({ id }: { id: number }) {
  return (
    <span
      className="inline-flex items-center gap-2 rounded-full border border-dashed bg-card px-3 py-1 text-xs text-muted-foreground"
      data-task-id={id}
      data-deleted="true"
    >
      <span className="line-through">Task #{id}</span>
      <Badge variant="outline" className="text-[10px]">
        deleted
      </Badge>
    </span>
  );
}
