import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { Log } from '@/types/run';
import { formatRelative, LogsTable } from './logs-table';

function log(partial: Partial<Log> & { id: number; message: string }): Log {
  return {
    run_id: 1,
    level: 'info',
    logged_at: '2026-05-21T10:00:00.500Z',
    ...partial,
  };
}

describe('formatRelative', () => {
  it('returns +seconds for positive deltas', () => {
    expect(formatRelative('2026-05-21T10:00:01.234Z', '2026-05-21T10:00:00.000Z')).toBe('+1.234s');
  });

  it('returns sub-second offsets with three decimal places', () => {
    expect(formatRelative('2026-05-21T10:00:00.123Z', '2026-05-21T10:00:00.000Z')).toBe('+0.123s');
  });

  it('returns empty when no start time supplied', () => {
    expect(formatRelative('2026-05-21T10:00:00.000Z')).toBe('');
  });
});

describe('LogsTable', () => {
  const logs: Log[] = [
    log({
      id: 1,
      level: 'info',
      message: 'starting run',
      logged_at: '2026-05-21T10:00:00.100Z',
    }),
    log({
      id: 2,
      level: 'warn',
      message: 'rate limited',
      logged_at: '2026-05-21T10:00:00.500Z',
    }),
    log({
      id: 3,
      level: 'error',
      message: 'aborted',
      logged_at: '2026-05-21T10:00:01.000Z',
    }),
  ];

  it('renders one row per log', () => {
    render(<LogsTable logs={logs} startedAt="2026-05-21T10:00:00.000Z" />);
    expect(screen.getByText('starting run')).toBeTruthy();
    expect(screen.getByText('rate limited')).toBeTruthy();
    expect(screen.getByText('aborted')).toBeTruthy();
  });

  it('filters rows by the case-insensitive substring of the message', () => {
    render(<LogsTable logs={logs} startedAt="2026-05-21T10:00:00.000Z" />);
    const input = screen.getByLabelText('Filter logs') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'RATE' } });
    expect(screen.queryByText('starting run')).toBeNull();
    expect(screen.getByText('rate limited')).toBeTruthy();
    expect(screen.queryByText('aborted')).toBeNull();
  });

  it('shows an empty hint when the filter matches no entries', () => {
    render(<LogsTable logs={logs} startedAt="2026-05-21T10:00:00.000Z" />);
    fireEvent.change(screen.getByLabelText('Filter logs'), { target: { value: 'no-match' } });
    expect(screen.getByText('No log entries match the filter.')).toBeTruthy();
  });

  it('renders the empty placeholder when there are zero logs', () => {
    render(<LogsTable logs={[]} startedAt="2026-05-21T10:00:00.000Z" />);
    expect(screen.getByText('No log entries yet.')).toBeTruthy();
  });
});
