/**
 * The Excalidraw canvas theme, independent of the app theme.
 *
 * Excalidraw's dark mode inverts brightness, so drawings made on a light
 * canvas (the Excalidraw / Obsidian default) look noticeably different in
 * dark mode. The canvas therefore defaults to light and the user flips it
 * with Excalidraw's own menu toggle (or Shift+Alt+D); the choice is
 * remembered per browser. Excalidraw doesn't persist theme in the scene, so
 * there's no per-drawing setting to honour.
 */

import { useCallback, useRef, useState } from 'react';

export type CanvasTheme = 'light' | 'dark';

export const CANVAS_THEME_STORAGE_KEY = 'tt:drawing-theme';

export function readCanvasTheme(): CanvasTheme {
  return window.localStorage.getItem(CANVAS_THEME_STORAGE_KEY) === 'dark' ? 'dark' : 'light';
}

export function useCanvasTheme(): [CanvasTheme, (next: CanvasTheme) => void] {
  const [theme, setThemeState] = useState<CanvasTheme>(readCanvasTheme);
  // Called from Excalidraw's onChange (every pointer move), so skip the
  // storage write unless the theme actually changed.
  const current = useRef(theme);
  const setTheme = useCallback((next: CanvasTheme) => {
    if (next === current.current) return;
    current.current = next;
    window.localStorage.setItem(CANVAS_THEME_STORAGE_KEY, next);
    setThemeState(next);
  }, []);
  return [theme, setTheme];
}
