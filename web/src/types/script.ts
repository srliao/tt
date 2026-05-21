/**
 * TypeScript types mirroring `internal/script/types.go`.
 */

export type ScheduleKind = 'every_tick' | 'daily' | 'weekly' | 'monthly';

export type Weekday =
  | 'monday'
  | 'tuesday'
  | 'wednesday'
  | 'thursday'
  | 'friday'
  | 'saturday'
  | 'sunday';

/**
 * Monthly day-of-month: either an integer 1..31 or the string literal "last".
 */
export type MonthlyDay = number | 'last';

/**
 * Discriminated union mirroring the parsed Go Schedule. The wire shape always
 * carries a `kind`; `weekday`/`day` are only meaningful for the matching kind.
 */
export type Schedule =
  | { kind: 'every_tick' }
  | { kind: 'daily' }
  | { kind: 'weekly'; weekday: Weekday }
  | { kind: 'monthly'; day: MonthlyDay };

export interface Script {
  id: number;
  name: string;
  code: string;
  enabled: boolean;
  schedule: Schedule;
  /** RFC3339 timestamp; omitted when the script has never run. */
  last_run_at?: string | null;
  /** RFC3339 timestamp. */
  created_at: string;
  /** RFC3339 timestamp. */
  updated_at: string;
}

export interface ScriptCreateInput {
  name: string;
  code: string;
  enabled: boolean;
  schedule: Schedule;
}

export type ScriptUpdateInput = ScriptCreateInput;
