/**
 * Tests for <ShortcutCheatsheet>.
 *
 * The cheatsheet is the single source of truth for discoverable keyboard
 * shortcuts. These tests guarantee the three documented groups are always
 * rendered and that every shortcut in the spec is listed.
 */

import { act, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { SHORTCUT_GROUPS, ShortcutCheatsheet } from './shortcut-cheatsheet';

function openCheatsheet() {
  act(() => {
    window.dispatchEvent(new Event('tt:toggle-cheatsheet'));
  });
}

describe('SHORTCUT_GROUPS', () => {
  it('exposes the four documented sections in order', () => {
    expect(SHORTCUT_GROUPS.map((g) => g.title)).toEqual([
      'Global',
      'Navigate',
      'On a task',
      'Selection',
    ]);
  });

  it('includes every documented shortcut', () => {
    const keys = SHORTCUT_GROUPS.flatMap((g) => g.items.map((i) => i.keys));
    for (const expected of [
      '⌘ K',
      '/',
      'n',
      '?',
      'g t',
      'g s',
      'g c',
      'g g',
      'g r',
      'j / k',
      '⇧ j / ⇧ k',
      '↵',
      'e',
      'd',
      's',
      't',
      'x',
      '␣',
      '⌘ A',
      '⇧ ⌘ A',
      'Esc',
    ]) {
      expect(keys).toContain(expected);
    }
  });

  it('moves x and Space rows into the Selection group', () => {
    const onTask = SHORTCUT_GROUPS.find((g) => g.title === 'On a task');
    const selection = SHORTCUT_GROUPS.find((g) => g.title === 'Selection');
    expect(onTask?.items.some((i) => i.keys === 'x' || i.keys === '␣')).toBe(false);
    expect(selection?.items.some((i) => i.keys === 'x')).toBe(true);
    expect(selection?.items.some((i) => i.keys === '␣')).toBe(true);
  });
});

describe('<ShortcutCheatsheet>', () => {
  it('renders all four group headings when opened', () => {
    render(<ShortcutCheatsheet />);
    openCheatsheet();
    expect(screen.getByRole('heading', { name: 'Global', level: 3 })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Navigate', level: 3 })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'On a task', level: 3 })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Selection', level: 3 })).toBeTruthy();
  });

  it('renders the inline tag editor and select-all descriptions', () => {
    render(<ShortcutCheatsheet />);
    openCheatsheet();
    expect(screen.getByText('Edit tags inline (single) or bulk-tag (selection)')).toBeTruthy();
    expect(screen.getByText('Extend range')).toBeTruthy();
    expect(screen.getByText('Select all visible')).toBeTruthy();
    expect(screen.getByText('Select all matching the filter')).toBeTruthy();
  });
});
