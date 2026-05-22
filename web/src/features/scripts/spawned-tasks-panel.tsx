/**
 * "Spawned tasks" panel for the editor sidebar. Lists every task that has
 * this script's id in `spawned_by_script_id`. Each row links into the task
 * edit modal on the /tasks page.
 *
 * Pagination uses the `nextCursor` returned by the server. Server pagination
 * shape is documented in `internal/httpapi/scripts.go`. v1 keeps the loaded
 * pages in local state and concatenates them; switching the script id (via
 * the route key) is enough to reset.
 */

import { Link } from '@tanstack/react-router';
import { formatDistanceToNow, parseISO } from 'date-fns';
import { useEffect, useState } from 'react';
import { useSpawnedTasks } from '@/api/scripts';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type { Task, TaskState } from '@/types/task';

export interface SpawnedTasksPanelProps {
  scriptId: number;
}

const STATE_LABEL: Record<TaskState, string> = {
  not_done: 'Open',
  done: 'Done',
  cancelled: 'Cancelled',
};

const STATE_VARIANT: Record<TaskState, 'default' | 'secondary' | 'outline'> = {
  not_done: 'default',
  done: 'secondary',
  cancelled: 'outline',
};

interface PaginatedTasksResponse {
  /** The page itself. The server may return a flat array (v1 default) or
   *  this object — handled in the hook. */
  tasks?: Task[];
  next_cursor?: string | null;
}

/**
 * The server contract from §06 returns `Task[]` directly with a cursor in
 * a future iteration; we accept both shapes defensively. Exported helpers
 * keep the panel logic testable.
 */
export function unwrapTaskPage(raw: Task[] | PaginatedTasksResponse): {
  tasks: Task[];
  nextCursor: string | null;
} {
  if (Array.isArray(raw)) return { tasks: raw, nextCursor: null };
  return { tasks: raw.tasks ?? [], nextCursor: raw.next_cursor ?? null };
}

export function SpawnedTasksPanel({ scriptId }: SpawnedTasksPanelProps) {
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [pages, setPages] = useState<Task[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);

  const { data, isLoading, isFetching } = useSpawnedTasks(scriptId, {
    limit: 20,
    cursor,
  });

  // Whenever a new page arrives, append it. We key on cursor so we don't
  // accidentally double-append on a re-render.
  useEffect(() => {
    if (!data) return;
    // useSpawnedTasks returns Task[]; coerce defensively in case the server
    // shape changes later.
    const { tasks, nextCursor: nc } = unwrapTaskPage(
      data as unknown as Task[] | PaginatedTasksResponse,
    );
    setPages((prev) => {
      // First page: replace; subsequent: append.
      if (cursor === undefined) return tasks;
      return [...prev, ...tasks];
    });
    setNextCursor(nc);
  }, [data, cursor]);

  if (isLoading && pages.length === 0) {
    return <p className="text-xs text-muted-foreground">Loading spawned tasks…</p>;
  }
  if (pages.length === 0) {
    return <p className="text-xs text-muted-foreground">No spawned tasks yet.</p>;
  }

  return (
    <div className="flex flex-col gap-2">
      <ul className="flex flex-col gap-1" aria-label="Spawned tasks">
        {pages.map((task) => (
          <li
            key={task.id}
            className="flex items-center justify-between gap-2 rounded-md border bg-card px-2 py-1.5"
          >
            <div className="flex min-w-0 flex-col">
              <Link
                to="/tasks"
                className="truncate text-xs font-medium hover:underline"
                aria-label={`Open task ${task.title}`}
              >
                {task.title}
              </Link>
              <span className="text-[10px] text-muted-foreground">
                {formatDistanceToNow(parseISO(task.created_at), { addSuffix: true })}
              </span>
            </div>
            <Badge variant={STATE_VARIANT[task.state]} className="text-[10px]">
              {STATE_LABEL[task.state]}
            </Badge>
          </li>
        ))}
      </ul>
      {nextCursor && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isFetching}
          onClick={() => setCursor(nextCursor)}
        >
          {isFetching ? 'Loading…' : 'Load more'}
        </Button>
      )}
    </div>
  );
}
