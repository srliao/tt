import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { CANVAS_THEME_STORAGE_KEY, readCanvasTheme, useCanvasTheme } from './canvas-theme';

afterEach(() => window.localStorage.clear());

describe('canvas theme', () => {
  it('defaults to light regardless of the app theme', () => {
    window.localStorage.setItem('theme', 'dark');
    expect(readCanvasTheme()).toBe('light');
  });

  it('ignores unknown stored values', () => {
    window.localStorage.setItem(CANVAS_THEME_STORAGE_KEY, 'sepia');
    expect(readCanvasTheme()).toBe('light');
  });

  it('remembers the chosen theme', () => {
    const { result } = renderHook(() => useCanvasTheme());
    act(() => result.current[1]('dark'));
    expect(result.current[0]).toBe('dark');
    expect(readCanvasTheme()).toBe('dark');
  });
});
