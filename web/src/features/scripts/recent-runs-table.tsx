/**
 * Recent runs table for a single script. Polls every 5s (configured in the
 * `useScriptRuns` hook) so the user sees status flip from `running` →
 * `ok`/`error` without manual refreshing.
 *
 * Columns: status pill, started_at (relative + absolute on hover), duration
 * (finished_at − started_at), spawned task count, link to `/runs/$id`.
 */

import { Link } from '@tanstack/react-router';
import { differenceInMilliseconds, format, formatDistanceToNow, parseISO } from 'date-fns';
import { useScriptRuns } from '@/api/scripts';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { Run, RunStatus } from '@/types/run';

export interface RecentRunsTableProps {
  scriptId: number;
}

/** Map run statuses to a Badge variant. Exported so the test can pin it. */
export function statusVariant(
  status: RunStatus,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (status) {
    case 'ok':
      return 'secondary';
    case 'error':
    case 'timeout':
      return 'destructive';
    case 'running':
      return 'default';
    default:
      return 'outline';
  }
}

/** Human-friendly duration in ms; "—" while still running. */
export function formatDuration(run: Pick<Run, 'started_at' | 'finished_at'>): string {
  if (!run.finished_at) return '—';
  try {
    const ms = differenceInMilliseconds(parseISO(run.finished_at), parseISO(run.started_at));
    if (ms < 1000) return `${ms} ms`;
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
    return `${Math.round(ms / 60_000)} min`;
  } catch {
    return '—';
  }
}

export function RecentRunsTable({ scriptId }: RecentRunsTableProps) {
  const { data: runs = [], isLoading } = useScriptRuns(scriptId, { limit: 20 });

  if (isLoading) {
    return <p className="text-xs text-muted-foreground">Loading runs…</p>;
  }
  if (runs.length === 0) {
    return <p className="text-xs text-muted-foreground">No runs yet.</p>;
  }

  return (
    <TooltipProvider>
      <table aria-label="Recent runs" className="w-full text-xs">
        <thead className="text-muted-foreground">
          <tr className="border-b">
            <th className="px-2 py-1.5 text-left font-medium">Status</th>
            <th className="px-2 py-1.5 text-left font-medium">Started</th>
            <th className="px-2 py-1.5 text-left font-medium">Duration</th>
            <th className="px-2 py-1.5 text-left font-medium">Tasks</th>
            <th className="w-12 px-2 py-1.5" />
          </tr>
        </thead>
        <tbody>
          {runs.map((run) => (
            <RunRow key={run.id} run={run} />
          ))}
        </tbody>
      </table>
    </TooltipProvider>
  );
}

function RunRow({ run }: { run: Run }) {
  const started = parseISO(run.started_at);
  return (
    <tr data-run-id={run.id} className="border-b hover:bg-muted/30">
      <td className="px-2 py-1.5 align-middle">
        <Badge variant={statusVariant(run.status)} className="text-[10px]">
          {run.status}
        </Badge>
      </td>
      <td className="px-2 py-1.5 align-middle">
        <Tooltip>
          <TooltipTrigger asChild>
            <span>{formatDistanceToNow(started, { addSuffix: true })}</span>
          </TooltipTrigger>
          <TooltipContent>{format(started, 'PPpp')}</TooltipContent>
        </Tooltip>
      </td>
      <td className="px-2 py-1.5 align-middle">{formatDuration(run)}</td>
      <td className="px-2 py-1.5 align-middle">{run.spawned_task_ids.length}</td>
      <td className="px-2 py-1.5 align-middle">
        <Link
          to="/runs/$id"
          params={{ id: String(run.id) }}
          className="text-xs text-primary hover:underline"
        >
          View
        </Link>
      </td>
    </tr>
  );
}
