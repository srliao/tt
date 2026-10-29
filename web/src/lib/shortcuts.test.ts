import { afterEach, describe, expect, it } from 'vitest';
import { isShortcutOptOut } from './shortcuts';

afterEach(() => {
  document.body.innerHTML = '';
});

describe('isShortcutOptOut', () => {
  it('matches targets inside a data-app-shortcuts="off" subtree', () => {
    document.body.innerHTML = '<div data-app-shortcuts="off"><canvas id="t"></canvas></div>';
    expect(isShortcutOptOut(document.getElementById('t'))).toBe(true);
  });

  it('matches Excalidraw dialogs portaled to document.body', () => {
    document.body.innerHTML =
      '<div class="excalidraw excalidraw-modal-container"><button id="t">Close</button></div>';
    expect(isShortcutOptOut(document.getElementById('t'))).toBe(true);
  });

  it('leaves the rest of the app alone', () => {
    document.body.innerHTML = '<main><button id="t">Add</button></main>';
    expect(isShortcutOptOut(document.getElementById('t'))).toBe(false);
    expect(isShortcutOptOut(window)).toBe(false);
  });
});
