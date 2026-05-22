/**
 * /runs/$id — detail view for a single script run.
 *
 * Layout:
 *  - Header line: linked script name, status pill, trigger, start/finish
 *    timestamps, total duration.
 *  - Error block (only when `status !== 'ok'`): the raw `error_message` in a
 *    monospace `<pre>` so stack traces stay readable.
 *  - `<LogsTable />` for the streamed log lines.
 *  - `<SpawnedTasksChips />` when at least one task was queued.
 *
 * While the run is still in-flight (`status === 'running'`) the page polls
 * the detail endpoint every 2s; once a terminal status arrives the polling
 * stops automatically so we don't keep hitting the server forever.
 */

import { Link } from '@tanstack/react-router';
import { format, parseISO } from 'date-fns';
import { ArrowLeftIcon } from 'lucide-react';
import { useRun } from '@/api/runs';
import { useScript } from '@/api/scripts';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import type { RunStatus } from '@/types/run';
import { formatDuration } from './list-page';
import { LogsTable } from './logs-table';
import { SpawnedTasksChips } from './spawned-tasks-chips';
import { StatusPill } from './status-pill';

/** Returns 2000 (ms) while the run is in-flight, false once terminal. */
export function pollInterval(status: RunStatus | undefined): number | false {
  return status === 'running' ? 2000 : false;
}

export interface RunDetailPageProps {
  id: number;
}

export function RunDetailPage({ id }: RunDetailPageProps) {
  // refetchInterval as a function: re-evaluated each tick so polling stops
  // automatically as soon as the server reports a terminal status.
  const { data, isLoading, error } = useRun(id, {
    refetchInterval: (query) => pollInterval(query.state.data?.status),
  });

  if (isLoading) {
    return (
      <section className="mx-auto w-full max-w-5xl px-4 py-4">
        <p className="text-sm text-muted-foreground">Loading run…</p>
      </section>
    );
  }
  if (error || !data) {
    return (
      <section className="mx-auto w-full max-w-5xl px-4 py-4">
        <p className="text-sm text-destructive">
          {(error as Error | null)?.message ?? 'Run not found.'}
        </p>
        <Button asChild variant="link" size="sm">
          <Link to="/runs">
            <ArrowLeftIcon /> Back to runs
          </Link>
        </Button>
      </section>
    );
  }

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-4">
      <Header runId={data.id} scriptId={data.script_id} />
      <DetailHeader
        status={data.status}
        trigger={data.trigger}
        startedAt={data.started_at}
        finishedAt={data.finished_at ?? null}
      />
      {data.status !== 'ok' && data.error_message && (
        <pre className="overflow-x-auto rounded border border-destructive/40 bg-destructive/5 p-3 font-mono text-xs text-destructive">
          {data.error_message}
        </pre>
      )}

      <Separator />

      <section className="flex flex-col gap-2">
        <h2 className="font-heading text-sm font-medium uppercase tracking-wide text-muted-foreground">
          Logs ({data.logs.length})
        </h2>
        <LogsTable logs={data.logs} startedAt={data.started_at} />
      </section>

      {data.spawned_task_ids.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="font-heading text-sm font-medium uppercase tracking-wide text-muted-foreground">
            Spawned tasks ({data.spawned_task_ids.length})
          </h2>
          <SpawnedTasksChips spawnedIds={data.spawned_task_ids} tasks={data.spawned_tasks} />
        </section>
      )}
    </section>
  );
}

function Header({ runId, scriptId }: { runId: number; scriptId: number }) {
  const { data: script } = useScript(scriptId);
  return (
    <header className="flex items-center justify-between gap-2">
      <div className="flex items-center gap-3">
        <Button asChild variant="ghost" size="sm">
          <Link to="/runs">
            <ArrowLeftIcon /> Runs
          </Link>
        </Button>
        <h1 className="font-heading text-xl font-medium">Run #{runId}</h1>
        <span className="text-sm text-muted-foreground">
          {script ? (
            <Link to="/scripts/$id" params={{ id: String(scriptId) }} className="hover:underline">
              {script.name}
            </Link>
          ) : (
            `Script #${scriptId}`
          )}
        </span>
      </div>
    </header>
  );
}

interface DetailHeaderProps {
  status: RunStatus;
  trigger: string;
  startedAt: string;
  finishedAt: string | null;
}

function DetailHeader({ status, trigger, startedAt, finishedAt }: DetailHeaderProps) {
  const started = parseISO(startedAt);
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 rounded border bg-card px-4 py-3 text-sm">
      <StatusPill status={status} />
      <div>
        <span className="text-xs text-muted-foreground">Trigger</span>{' '}
        <span className="capitalize">{trigger}</span>
      </div>
      <div>
        <span className="text-xs text-muted-foreground">Started</span>{' '}
        <span title={format(started, 'PPpp')}>{format(started, 'PPpp')}</span>
      </div>
      <div>
        <span className="text-xs text-muted-foreground">Finished</span>{' '}
        <span>{finishedAt ? format(parseISO(finishedAt), 'PPpp') : '—'}</span>
      </div>
      <div>
        <span className="text-xs text-muted-foreground">Duration</span>{' '}
        <span>{formatDuration({ started_at: startedAt, finished_at: finishedAt })}</span>
      </div>
    </div>
  );
}
