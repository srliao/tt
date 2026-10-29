import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SAVE_DEBOUNCE_MS, useSceneAutosave } from './use-scene-autosave';

/** A save fn whose calls resolve/reject only when the test says so. */
function controlledSave() {
  const calls: { scene: string; resolve: () => void; reject: () => void }[] = [];
  const save = vi.fn(
    (scene: string) =>
      new Promise<void>((resolve, reject) => {
        calls.push({ scene, resolve, reject });
      }),
  );
  return { save, calls };
}

async function settle(fn: () => void) {
  await act(async () => {
    fn();
    await Promise.resolve();
  });
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

function setup() {
  const { save, calls } = controlledSave();
  const hook = renderHook(() => useSceneAutosave(save));
  const change = (scene: string) =>
    act(() => {
      hook.result.current.notifyChange(() => scene);
    });
  const tick = () =>
    act(() => {
      vi.advanceTimersByTime(SAVE_DEBOUNCE_MS);
    });
  return { hook, save, calls, change, tick };
}

describe('useSceneAutosave', () => {
  it('records the baseline without saving until the user interacts', () => {
    const { hook, save, change, tick } = setup();
    change('seed');
    tick();
    expect(save).not.toHaveBeenCalled();

    act(() => hook.result.current.markInteracted());
    change('seed');
    tick();
    expect(save).not.toHaveBeenCalled();
    expect(hook.result.current.status).toBe('saved');
  });

  it('debounces and saves the latest scene', async () => {
    const { hook, save, calls, change, tick } = setup();
    change('seed');
    tick();
    act(() => hook.result.current.markInteracted());

    change('a');
    change('b');
    expect(hook.result.current.status).toBe('pending');
    tick();
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenLastCalledWith('b');
    expect(hook.result.current.status).toBe('saving');

    await settle(() => calls[0].resolve());
    expect(hook.result.current.status).toBe('saved');
  });

  it('keeps one save in flight and follows up with the newest scene', async () => {
    const { hook, save, calls, change, tick } = setup();
    change('seed');
    tick();
    act(() => hook.result.current.markInteracted());

    change('a');
    tick();
    change('b');
    tick();
    change('c');
    tick();
    expect(save).toHaveBeenCalledTimes(1);

    await settle(() => calls[0].resolve());
    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenLastCalledWith('c');
  });

  it('flushes unsaved work on unmount, including after a failed save', async () => {
    const { hook, save, calls, change, tick } = setup();
    change('seed');
    tick();
    act(() => hook.result.current.markInteracted());

    change('a');
    tick();
    await settle(() => calls[0].reject());
    expect(hook.result.current.status).toBe('error');

    hook.unmount();
    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenLastCalledWith('a');
  });

  it('warns on tab close only while something is unsaved', () => {
    const { hook, change, tick } = setup();
    const fire = () => {
      const e = new Event('beforeunload', { cancelable: true });
      window.dispatchEvent(e);
      return e.defaultPrevented;
    };
    change('seed');
    tick();
    expect(fire()).toBe(false);

    act(() => hook.result.current.markInteracted());
    change('a');
    expect(fire()).toBe(true);
  });
});
