/**
 * TypeScript types mirroring `internal/task/types.go`.
 *
 * JSON timestamps are RFC3339 strings (encoded via Go's `time.Time` default).
 * Optional Go fields (pointer types) become `T | null` per `omitempty`-less
 * defaults on the wire.
 */

export type TaskState = 'not_done' | 'done' | 'cancelled';

export type TaskDueRange = '' | 'overdue' | 'today' | 'this_week' | 'none';

export type TaskSortAxis = 'priority' | 'due_date' | 'created_at' | 'title';

export interface Task {
  id: number;
  title: string;
  notes: string;
  state: TaskState;
  /** ISO date (YYYY-MM-DD) or null. */
  due_date: string | null;
  priority: number;
  /** Fractional position in the staged batch; null when not staged. */
  staged_order: number | null;
  spawned_by_script_id: number | null;
  /** RFC3339 timestamp. */
  created_at: string;
  /** RFC3339 timestamp; null while still actionable. */
  completed_at: string | null;
  /** RFC3339 timestamp; null while still actionable. */
  cancelled_at: string | null;
  /** RFC3339 timestamp. */
  updated_at: string;
  tags: string[];
}

export interface TaskCreateInput {
  title: string;
  notes?: string;
  due_date?: string | null;
  tags?: string[];
  spawned_by_script_id?: number | null;
}

export interface TaskUpdateInput {
  title: string;
  notes: string;
  due_date: string | null;
  tags: string[];
}
