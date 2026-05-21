/**
 * TypeScript types mirroring `internal/script/types.go` (Run and Log).
 */

export type RunTrigger = 'scheduled' | 'manual';

export type RunStatus = 'running' | 'ok' | 'error' | 'timeout';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface Run {
  id: number;
  script_id: number;
  /** RFC3339 timestamp. */
  started_at: string;
  /** RFC3339 timestamp; omitted while running. */
  finished_at?: string | null;
  status: RunStatus;
  error_message: string;
  spawned_task_ids: number[];
  trigger: RunTrigger;
}

export interface Log {
  id: number;
  run_id: number;
  level: LogLevel;
  message: string;
  /** RFC3339 timestamp. */
  logged_at: string;
}
