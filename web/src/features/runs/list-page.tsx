/**
 * The /runs page: a global, filterable log of every script run.
 *
 * Filter state is reflected in the URL search params so a particular query
 * can be shared/bookmarked. Three filters live in the bar:
 *
 *  - script (any | one of the registered scripts).
 *  - status (any | running | ok | error | timeout).
 *  - from/to (YYYY-MM-DD date range, inclusive on `started_at`).
 *
 * Pagination uses the server's `cursor` offset: when the page length equals
 * the request limit, we append a "Load more" button that bumps a local
 * pageSize and reissues the query. Filter changes reset the pageSize to the
 * default to avoid pulling a giant slice for narrow queries.
 *
 * The empty state copy comes verbatim from spec §6 — keep it stable so users
 * who memorize the message can search for it.
 */

import { Link, useNavigate, useSearch } from '@tanstack/react-router';
import { differenceInMilliseconds, format, formatDistanceToNow, parseISO } from 'date-fns';
import { useCallback, useMemo, useState } from 'react';
import { z } from 'zod';
import type { RunListFilters } from '@/api/runs';
import { useRuns } from '@/api/runs';
import { useScripts } from '@/api/scripts';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { Run, RunStatus } from '@/types/run';
import { StatusPill } from './status-pill';

export const RUN_STATUSES = ['running', 'ok', 'error', 'timeout'] as const;

export const runsSearchSchema = z
  .object({
    script_id: z.coerce.number().int().positive().optional(),
    status: z.enum(RUN_STATUSES).optional(),
    from: z.string().optional(),
    to: z.string().optional(),
  })
  .partial();

export type RunsSearch = z.infer<typeof runsSearchSchema>;

const ANY_VALUE = '__any__';
const PAGE_SIZE = 25;

/**
 * Map the URL search params + a local pageSize to the request filters
 * (RFC3339 from/to bounds and a non-zero limit). Exported for tests.
 */
export function toRunListFilters(search: RunsSearch, pageSize: number): RunListFilters {
  const filters: RunListFilters = { limit: pageSize };
  if (search.script_id !== undefined) filters.script_id = search.script_id;
  if (search.status) filters.status = search.status;
  if (search.from) filters.from = `${search.from}T00:00:00Z`;
  if (search.to) filters.to = `${search.to}T23:59:59Z`;
  return filters;
}

/** Human-friendly duration formatter (also used by the detail header). */
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

export function RunsListPage() {
  // strict:false so unit tests can mount this without registering the file route.
  const search = useSearch({ strict: false }) as RunsSearch;
  const navigate = useNavigate();
  const [pageSize, setPageSize] = useState(PAGE_SIZE);

  const setSearch = useCallback(
    (updates: Partial<RunsSearch>) => {
      // Filter changes always reset the page size — pulling a larger slice
      // after the user re-narrows the query would mask the new filters.
      setPageSize(PAGE_SIZE);
      void navigate({
        to: '.',
        search: (prev) => {
          const merged: RunsSearch = { ...(prev as RunsSearch), ...updates };
          const cleaned: RunsSearch = {};
          for (const [key, value] of Object.entries(merged)) {
            if (value === undefined || value === '' || value === null) continue;
            (cleaned as Record<string, unknown>)[key] = value;
          }
          return cleaned;
        },
      });
    },
    [navigate],
  );

  const { data: scripts = [] } = useScripts();
  const scriptName = useMemo(() => {
    const map = new Map<number, string>();
    for (const s of scripts) map.set(s.id, s.name);
    return map;
  }, [scripts]);

  const filters = useMemo(() => toRunListFilters(search, pageSize), [search, pageSize]);
  const { data: runs = [], isLoading } = useRuns(filters);

  const hasMore = runs.length >= pageSize;

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-4">
      <header>
        <h1 className="font-heading text-xl font-medium">Runs</h1>
      </header>

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="runs-script">Script</Label>
          <Select
            value={search.script_id ? String(search.script_id) : ANY_VALUE}
            onValueChange={(v) => setSearch({ script_id: v === ANY_VALUE ? undefined : Number(v) })}
          >
            <SelectTrigger id="runs-script" className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY_VALUE}>Any script</SelectItem>
              {scripts.map((s) => (
                <SelectItem key={s.id} value={String(s.id)}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1">
          <Label htmlFor="runs-status">Status</Label>
          <Select
            value={search.status ?? ANY_VALUE}
            onValueChange={(v) =>
              setSearch({ status: v === ANY_VALUE ? undefined : (v as RunStatus) })
            }
          >
            <SelectTrigger id="runs-status" className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY_VALUE}>Any status</SelectItem>
              {RUN_STATUSES.map((s) => (
                <SelectItem key={s} value={s}>
                  {s}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1">
          <Label htmlFor="runs-from">From</Label>
          <Input
            id="runs-from"
            type="date"
            className="w-40"
            value={search.from ?? ''}
            onChange={(e) => setSearch({ from: e.target.value || undefined })}
          />
        </div>

        <div className="flex flex-col gap-1">
          <Label htmlFor="runs-to">To</Label>
          <Input
            id="runs-to"
            type="date"
            className="w-40"
            value={search.to ?? ''}
            onChange={(e) => setSearch({ to: e.target.value || undefined })}
          />
        </div>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading runs…</p>
      ) : runs.length === 0 ? (
        <EmptyState />
      ) : (
        <>
          <Table aria-label="Runs">
            <TableHeader>
              <TableRow>
                <TableHead>Script</TableHead>
                <TableHead>Started</TableHead>
                <TableHead>Duration</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Tasks</TableHead>
                <TableHead>Trigger</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs.map((run) => (
                <RunListRow
                  key={run.id}
                  run={run}
                  scriptName={scriptName.get(run.script_id) ?? `#${run.script_id}`}
                />
              ))}
            </TableBody>
          </Table>
          {hasMore && (
            <div className="flex justify-center">
              <Button variant="outline" size="sm" onClick={() => setPageSize((n) => n + PAGE_SIZE)}>
                Load more
              </Button>
            </div>
          )}
        </>
      )}
    </section>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-12 text-center">
      <h2 className="font-heading text-lg">No script runs yet</h2>
      <p className="max-w-md text-sm text-muted-foreground">
        Manually trigger a script or wait for its schedule.
      </p>
      <Button asChild variant="outline" size="sm">
        <Link to="/scripts">Go to scripts</Link>
      </Button>
    </div>
  );
}

interface RunListRowProps {
  run: Run;
  scriptName: string;
}

function RunListRow({ run, scriptName }: RunListRowProps) {
  const started = parseISO(run.started_at);
  return (
    <TableRow data-run-id={run.id}>
      <TableCell>
        <Link
          to="/scripts/$id"
          params={{ id: String(run.script_id) }}
          className="text-sm font-medium hover:underline"
          onClick={(e) => e.stopPropagation()}
        >
          {scriptName}
        </Link>
      </TableCell>
      <TableCell>
        <Link
          to="/runs/$id"
          params={{ id: String(run.id) }}
          className="text-sm hover:underline"
          title={format(started, 'PPpp')}
        >
          {formatDistanceToNow(started, { addSuffix: true })}
        </Link>
      </TableCell>
      <TableCell className="text-sm">{formatDuration(run)}</TableCell>
      <TableCell>
        <StatusPill status={run.status} />
      </TableCell>
      <TableCell className="text-sm">{run.spawned_task_ids.length}</TableCell>
      <TableCell className="text-sm capitalize">{run.trigger}</TableCell>
    </TableRow>
  );
}
