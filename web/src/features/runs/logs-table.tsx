/**
 * Logs panel for the run detail page.
 *
 * Each log row shows:
 *   - Relative time, e.g. "+0.123s" from the run's `started_at` (falls back to
 *     the first log's `logged_at` when the run hasn't recorded a start time
 *     yet — should be rare, but defensive against partial data).
 *   - Absolute time (HH:mm:ss.SSS) for grokking ordering when relative times
 *     compress into the same bucket.
 *   - Level badge (debug / info / warn / error) with colour cues.
 *   - The message text in a monospace block that preserves whitespace.
 *
 * A filter input above the table does a case-insensitive substring match
 * against the message. The table also auto-scrolls to the latest entry on
 * first render and after each update; once the user scrolls upward the
 * autoscroll is paused (and surfaced via a "Resume auto-scroll" button) so
 * incoming log lines from a still-running script don't yank them away from
 * what they were reading.
 */

import { differenceInMilliseconds, format, parseISO } from 'date-fns';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import type { Log, LogLevel } from '@/types/run';

export interface LogsTableProps {
  logs: Log[];
  /** RFC3339 start time used to compute relative offsets. Optional. */
  startedAt?: string;
}

const LEVEL_CLASSES: Record<LogLevel, string> = {
  debug: 'bg-muted text-muted-foreground',
  info: 'bg-sky-100 text-sky-900 dark:bg-sky-900/40 dark:text-sky-200',
  warn: 'bg-amber-100 text-amber-900 dark:bg-amber-900/40 dark:text-amber-200',
  error: 'bg-destructive/10 text-destructive',
};

/** Returns "+0.123s" given a log entry and the run's start time. */
export function formatRelative(logged: string, start?: string): string {
  if (!start) return '';
  try {
    const ms = differenceInMilliseconds(parseISO(logged), parseISO(start));
    const sign = ms < 0 ? '-' : '+';
    const abs = Math.abs(ms);
    if (abs < 1000) return `${sign}0.${String(abs).padStart(3, '0')}s`;
    return `${sign}${(abs / 1000).toFixed(3)}s`;
  } catch {
    return '';
  }
}

export function LogsTable({ logs, startedAt }: LogsTableProps) {
  const [filter, setFilter] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    if (!needle) return logs;
    return logs.filter((log) => log.message.toLowerCase().includes(needle));
  }, [logs, filter]);

  // Auto-scroll on every update while the user hasn't manually scrolled up.
  // We track `autoScroll` separately so a single upward gesture pins the view.
  // biome-ignore lint/correctness/useExhaustiveDependencies: scroll target depends on filtered.length and autoScroll flag.
  useEffect(() => {
    if (!autoScroll) return;
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [filtered.length, autoScroll]);

  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    // Allow a small tolerance — browsers don't always land on an exact pixel
    // boundary. ~16px keeps "near the bottom" interactions sticky.
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 16;
    setAutoScroll(atBottom);
  };

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <Input
          type="search"
          placeholder="Filter logs by message…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="max-w-sm"
          aria-label="Filter logs"
        />
        {!autoScroll && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setAutoScroll(true);
              const el = scrollRef.current;
              if (el) el.scrollTop = el.scrollHeight;
            }}
          >
            Resume auto-scroll
          </Button>
        )}
      </div>

      {filtered.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {logs.length === 0 ? 'No log entries yet.' : 'No log entries match the filter.'}
        </p>
      ) : (
        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="max-h-96 overflow-y-auto rounded border bg-muted/20"
          data-testid="logs-scroll"
        >
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-muted/80 text-muted-foreground">
              <tr className="border-b">
                <th className="px-2 py-1 text-left font-medium">+T</th>
                <th className="px-2 py-1 text-left font-medium">Time</th>
                <th className="px-2 py-1 text-left font-medium">Level</th>
                <th className="px-2 py-1 text-left font-medium">Message</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((log) => (
                <LogRow key={log.id} log={log} startedAt={startedAt} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function LogRow({ log, startedAt }: { log: Log; startedAt?: string }) {
  const logged = parseISO(log.logged_at);
  return (
    <tr data-log-id={log.id} className="border-b last:border-0 align-top">
      <td className="px-2 py-1 font-mono text-[11px] text-muted-foreground whitespace-nowrap">
        {formatRelative(log.logged_at, startedAt)}
      </td>
      <td className="px-2 py-1 font-mono text-[11px] text-muted-foreground whitespace-nowrap">
        {format(logged, 'HH:mm:ss.SSS')}
      </td>
      <td className="px-2 py-1">
        <Badge variant="outline" className={cn('text-[10px]', LEVEL_CLASSES[log.level])}>
          {log.level}
        </Badge>
      </td>
      <td className="px-2 py-1 font-mono whitespace-pre-wrap break-words">{log.message}</td>
    </tr>
  );
}
