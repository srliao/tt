/**
 * `useSelection` — sessionStorage-backed selection state for the task list.
 *
 * Phase 3 lifts selection out of `page.tsx` so it survives filter changes and
 * full page reloads within the same tab session. `sessionStorage` (not
 * `localStorage`) so a fresh tab starts empty and closing the tab drops the
 * working set entirely — selection is ephemeral working state, not user data.
 *
 * Phase 8 follow-up: selection is shared across every component that calls
 * the hook. The previous implementation kept state in `useState`, so two
 * callers (the task page and the command palette) maintained independent
 * copies — palette mutations updated sessionStorage but the page's
 * `BulkActionBar` never re-read it. We now back the hook with a
 * module-level store + `useSyncExternalStore`, giving every consumer the
 * same snapshot and notifying all subscribers on every write.
 *
 * The hook also exposes `visibleCount` / `offScreenCount` so the bulk-action
 * bar (Phase 4) can warn the user when their selection includes ids the
 * current filter has hidden.
 */
import { useCallback, useSyncExternalStore } from 'react';
import type { Task } from '@/types/task';

const STORAGE_KEY = 'tt:selection';

function readInitial(): Set<number> {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    if (!raw) return new Set();
    const arr = JSON.parse(raw) as number[];
    return new Set(Array.isArray(arr) ? arr : []);
  } catch {
    // Corrupted storage shouldn't blank the page — just start empty.
    return new Set();
  }
}

// Module-level store: one shared Set<number> for the whole tab. All
// `useSelection` consumers subscribe to the same snapshot so a mutation
// from any caller (page, palette, future surfaces) is observed everywhere.
let current: Set<number> = readInitial();
const listeners = new Set<() => void>();

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

function getSnapshot(): Set<number> {
  return current;
}

function write(next: Set<number>): void {
  current = next;
  try {
    if (next.size === 0) sessionStorage.removeItem(STORAGE_KEY);
    else sessionStorage.setItem(STORAGE_KEY, JSON.stringify([...next]));
  } catch {
    // sessionStorage can throw in private mode — selection just won't persist.
  }
  for (const l of listeners) l();
}

/**
 * Test-only escape hatch: re-hydrate the module-level store from
 * sessionStorage. Each test runs `sessionStorage.clear()` in its
 * `beforeEach`, but the store caches the value in `current` from the
 * very first module import — so without this helper, mutations from
 * one test bleed into the next, and tests that pre-seed sessionStorage
 * before mounting wouldn't see the seeded value.
 */
export function __resetSelectionStoreForTests(): void {
  current = readInitial();
  for (const l of listeners) l();
}

export interface UseSelectionResult {
  /** All selected task ids, regardless of filter visibility. */
  selected: Set<number>;
  /** How many selected ids are present in the currently-visible task list. */
  visibleCount: number;
  /** How many selected ids are not in the visible list (filter excluded them). */
  offScreenCount: number;
  toggle: (id: number) => void;
  add: (ids: number[] | Set<number>) => void;
  remove: (ids: number[] | Set<number>) => void;
  clear: () => void;
  setAll: (ids: number[] | Set<number>) => void;
  has: (id: number) => boolean;
}

export function useSelection(visibleTasks: Task[]): UseSelectionResult {
  const selected = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  const visibleIds = new Set(visibleTasks.map((t) => t.id));
  let visibleCount = 0;
  for (const id of selected) if (visibleIds.has(id)) visibleCount++;
  const offScreenCount = selected.size - visibleCount;

  const toggle = useCallback((id: number) => {
    const next = new Set(current);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    write(next);
  }, []);

  const add = useCallback((ids: number[] | Set<number>) => {
    const next = new Set(current);
    for (const id of ids) next.add(id);
    write(next);
  }, []);

  const remove = useCallback((ids: number[] | Set<number>) => {
    const next = new Set(current);
    for (const id of ids) next.delete(id);
    write(next);
  }, []);

  const clear = useCallback(() => write(new Set()), []);

  const setAll = useCallback((ids: number[] | Set<number>) => {
    write(new Set(ids));
  }, []);

  // `has` is a thin wrapper around selected.has — identity changes every render
  // once selection mutates. Callers that need stable identity (e.g. React.memo
  // children) should use selected.has(id) directly instead of plumbing this.
  const has = (id: number) => selected.has(id);

  return { selected, visibleCount, offScreenCount, toggle, add, remove, clear, setAll, has };
}
