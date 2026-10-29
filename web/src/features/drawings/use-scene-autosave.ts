/**
 * Debounced autosave for the Excalidraw canvas, kept free of Excalidraw so
 * it can be tested on its own.
 *
 * - Callers report every change with a lazy `getScene` (serializing is too
 *   expensive to do per pointer move); it only runs when a save is due.
 * - Until the user first interacts, flushes just record the baseline, which
 *   absorbs Excalidraw normalizing the seed without writing on open.
 * - Identical serializations are skipped.
 * - At most one save is in flight. A flush requested meanwhile runs once the
 *   current save settles, with the latest scene, so an older scene can never
 *   land after a newer one.
 * - Unmount flushes anything unsaved, including after a failed save. Tab
 *   close flushes too and asks the browser to warn, since a request started
 *   during unload may not complete.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

export type SaveStatus = 'saved' | 'pending' | 'saving' | 'error';

export const SAVE_DEBOUNCE_MS = 800;

export function useSceneAutosave(save: (scene: string) => Promise<unknown>) {
  const [status, setStatus] = useState<SaveStatus>('saved');

  const saveRef = useRef(save);
  saveRef.current = save;
  const getScene = useRef<(() => string) | null>(null);
  const lastSaved = useRef<string | null>(null);
  const interacted = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inFlight = useRef(false);
  const flushAgain = useRef(false);
  const failed = useRef(false);

  const flush = useCallback(function flushScene() {
    if (timer.current) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    const get = getScene.current;
    if (!get) return;
    if (inFlight.current) {
      flushAgain.current = true;
      return;
    }
    const scene = get();
    if (!interacted.current) {
      lastSaved.current = scene;
      return;
    }
    if (scene === lastSaved.current) {
      failed.current = false;
      setStatus('saved');
      return;
    }
    inFlight.current = true;
    setStatus('saving');
    saveRef.current(scene).then(
      () => settle(true),
      () => settle(false),
    );

    function settle(ok: boolean) {
      inFlight.current = false;
      failed.current = !ok;
      if (ok) lastSaved.current = scene;
      if (flushAgain.current) {
        flushAgain.current = false;
        flushScene();
        return;
      }
      setStatus(!ok ? 'error' : timer.current ? 'pending' : 'saved');
    }
  }, []);

  const notifyChange = useCallback(
    (get: () => string) => {
      getScene.current = get;
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(flush, SAVE_DEBOUNCE_MS);
      if (interacted.current) setStatus((s) => (s === 'saving' ? s : 'pending'));
    },
    [flush],
  );

  const markInteracted = useCallback(() => {
    interacted.current = true;
  }, []);

  useEffect(() => {
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (!timer.current && !inFlight.current && !failed.current) return;
      flush();
      e.preventDefault();
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => {
      window.removeEventListener('beforeunload', onBeforeUnload);
      if (interacted.current) flush();
    };
  }, [flush]);

  return { status, notifyChange, markInteracted };
}
