/**
 * `useSelection` — sessionStorage-backed selection state for the task list.
 *
 * Phase 3 lifts selection out of `page.tsx` so it survives filter changes and
 * full page reloads within the same tab session. `sessionStorage` (not
 * `localStorage`) so a fresh tab starts empty and closing the tab drops the
 * working set entirely — selection is ephemeral working state, not user data.
 *
 * The hook also exposes `visibleCount` / `offScreenCount` so the bulk-action
 * bar (Phase 4) can warn the user when their selection includes ids the
 * current filter has hidden.
 */
import { useCallback, useEffect, useState } from 'react';
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
  const [selected, setSelected] = useState<Set<number>>(readInitial);

  // Persist every change. We replace the storage value entirely each time
  // rather than diff — the set is small and the write is cheap.
  useEffect(() => {
    try {
      if (selected.size === 0) sessionStorage.removeItem(STORAGE_KEY);
      else sessionStorage.setItem(STORAGE_KEY, JSON.stringify([...selected]));
    } catch {
      // sessionStorage can throw in private mode — selection just won't persist.
    }
  }, [selected]);

  const visibleIds = new Set(visibleTasks.map((t) => t.id));
  let visibleCount = 0;
  for (const id of selected) if (visibleIds.has(id)) visibleCount++;
  const offScreenCount = selected.size - visibleCount;

  const toggle = useCallback((id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const add = useCallback((ids: number[] | Set<number>) => {
    setSelected((prev) => {
      const next = new Set(prev);
      for (const id of ids) next.add(id);
      return next;
    });
  }, []);

  const remove = useCallback((ids: number[] | Set<number>) => {
    setSelected((prev) => {
      const next = new Set(prev);
      for (const id of ids) next.delete(id);
      return next;
    });
  }, []);

  const clear = useCallback(() => setSelected(new Set()), []);

  const setAll = useCallback((ids: number[] | Set<number>) => {
    setSelected(new Set(ids));
  }, []);

  // `has` is a thin wrapper around selected.has — identity changes every render
  // once selection mutates. Callers that need stable identity (e.g. React.memo
  // children) should use selected.has(id) directly instead of plumbing this.
  const has = (id: number) => selected.has(id);

  return { selected, visibleCount, offScreenCount, toggle, add, remove, clear, setAll, has };
}
