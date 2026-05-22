import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import type { Task } from '@/types/task';
import { useSelection } from './use-selection';

function mkTask(id: number): Task {
  return {
    id,
    title: `t${id}`,
    notes: '',
    state: 'not_done',
    due_date: null,
    priority: 0,
    staged_order: null,
    spawned_by_script_id: null,
    created_at: '2026-01-01T00:00:00Z',
    completed_at: null,
  } as Task;
}

beforeEach(() => sessionStorage.clear());

describe('useSelection', () => {
  it('starts empty when sessionStorage is empty', () => {
    const { result } = renderHook(() => useSelection([]));
    expect(result.current.selected.size).toBe(0);
    expect(result.current.visibleCount).toBe(0);
    expect(result.current.offScreenCount).toBe(0);
  });

  it('toggle(id) adds then removes', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.toggle(1));
    expect(result.current.selected.has(1)).toBe(true);
    expect(result.current.selected.size).toBe(1);
    act(() => result.current.toggle(1));
    expect(result.current.selected.has(1)).toBe(false);
    expect(result.current.selected.size).toBe(0);
  });

  it('add is a union (no duplicates, existing kept)', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add([1, 2]));
    act(() => result.current.add([2, 3]));
    expect([...result.current.selected].sort()).toEqual([1, 2, 3]);
  });

  it('add accepts a Set', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add(new Set([4, 5])));
    expect([...result.current.selected].sort()).toEqual([4, 5]);
  });

  it('remove deletes only the listed ids', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add([1, 2, 3]));
    act(() => result.current.remove([2]));
    expect([...result.current.selected].sort()).toEqual([1, 3]);
  });

  it('clear empties the selection', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add([1, 2, 3]));
    act(() => result.current.clear());
    expect(result.current.selected.size).toBe(0);
  });

  it('setAll replaces wholesale', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add([1, 2, 3]));
    act(() => result.current.setAll([10, 20]));
    expect([...result.current.selected].sort((a, b) => a - b)).toEqual([10, 20]);
  });

  it('has reflects current membership', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add([7]));
    expect(result.current.has(7)).toBe(true);
    expect(result.current.has(8)).toBe(false);
  });

  it('hydrates from sessionStorage on mount', () => {
    sessionStorage.setItem('tt:selection', JSON.stringify([1, 2]));
    const { result } = renderHook(() => useSelection([]));
    expect([...result.current.selected].sort()).toEqual([1, 2]);
  });

  it('selection round-trips through unmount and remount', () => {
    const { result, unmount } = renderHook(() => useSelection([]));
    act(() => result.current.add([3, 7]));
    unmount();
    const { result: result2 } = renderHook(() => useSelection([]));
    expect([...result2.current.selected].sort((a, b) => a - b)).toEqual([3, 7]);
  });

  it('survives corrupted sessionStorage without throwing', () => {
    sessionStorage.setItem('tt:selection', 'not-json{');
    const { result } = renderHook(() => useSelection([]));
    expect(result.current.selected.size).toBe(0);
  });

  it('ignores non-array JSON payloads', () => {
    sessionStorage.setItem('tt:selection', JSON.stringify({ a: 1 }));
    const { result } = renderHook(() => useSelection([]));
    expect(result.current.selected.size).toBe(0);
  });

  it('persists changes to sessionStorage', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add([1, 2]));
    const raw = sessionStorage.getItem('tt:selection');
    expect(raw).not.toBeNull();
    expect(JSON.parse(raw as string).sort()).toEqual([1, 2]);
  });

  it('clears the sessionStorage key when selection drops to empty', () => {
    const { result } = renderHook(() => useSelection([]));
    act(() => result.current.add([1]));
    expect(sessionStorage.getItem('tt:selection')).not.toBeNull();
    act(() => result.current.clear());
    expect(sessionStorage.getItem('tt:selection')).toBeNull();
  });

  it('computes visibleCount and offScreenCount against the visible task list', () => {
    sessionStorage.setItem('tt:selection', JSON.stringify([1, 2, 3]));
    const { result } = renderHook(() => useSelection([mkTask(1), mkTask(4)]));
    expect(result.current.visibleCount).toBe(1);
    expect(result.current.offScreenCount).toBe(2);
  });

  it('reports zero off-screen when every selected id is visible', () => {
    const { result } = renderHook(() => useSelection([mkTask(1), mkTask(2), mkTask(3)]));
    act(() => result.current.add([1, 2]));
    expect(result.current.visibleCount).toBe(2);
    expect(result.current.offScreenCount).toBe(0);
  });
});
