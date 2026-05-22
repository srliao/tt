/**
 * Unit tests for <TagGlyph> / <TagGlyphList>.
 *
 * Wrap in <ThemeProvider> since the component reads `resolvedTheme` from
 * the theme context. Tooltip content is portaled — we assert on the
 * trigger button + aria-label rather than poking inside the portal.
 */

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ThemeProvider } from '@/components/theme-provider';
import { TAG_GLYPH_MAX, TagGlyph, TagGlyphList } from '@/components/ui/tag-glyph';
import { TooltipProvider } from '@/components/ui/tooltip';

function renderWithTheme(node: React.ReactNode) {
  return render(
    <ThemeProvider>
      <TooltipProvider>{node}</TooltipProvider>
    </ThemeProvider>,
  );
}

describe('<TagGlyph>', () => {
  it('renders a button with the given initial and aria-label', () => {
    renderWithTheme(<TagGlyph name="backend" initial="B" />);
    const btn = screen.getByRole('button', { name: 'Tag backend' });
    expect(btn.textContent).toBe('B');
  });

  it('fires onClick with the click event when clicked', () => {
    const onClick = vi.fn();
    renderWithTheme(<TagGlyph name="work" initial="W" onClick={onClick} />);
    const btn = screen.getByRole('button', { name: 'Tag work' });
    fireEvent.click(btn);
    expect(onClick).toHaveBeenCalledTimes(1);
    expect(onClick.mock.calls[0][0]).toBeDefined();
  });
});

describe('<TagGlyphList>', () => {
  it('renders one button per visible tag with the correct initial', () => {
    const initialMap = new Map<string, string>([
      ['backend', 'Ba'],
      ['build', 'Bu'],
    ]);
    renderWithTheme(
      <TagGlyphList tags={['backend', 'build']} initialMap={initialMap} onTagClick={() => {}} />,
    );
    const backend = screen.getByRole('button', { name: 'Tag backend' });
    const build = screen.getByRole('button', { name: 'Tag build' });
    expect(backend.textContent).toBe('Ba');
    expect(build.textContent).toBe('Bu');
  });

  it('invokes onTagClick with the tag name and the click event', () => {
    const onTagClick = vi.fn();
    renderWithTheme(
      <TagGlyphList tags={['ops']} initialMap={new Map([['ops', 'O']])} onTagClick={onTagClick} />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Tag ops' }));
    expect(onTagClick).toHaveBeenCalledTimes(1);
    expect(onTagClick.mock.calls[0][0]).toBe('ops');
    expect(onTagClick.mock.calls[0][1]).toBeDefined();
  });

  it('forwards shift-click modifier flags via the event', () => {
    const onTagClick = vi.fn();
    renderWithTheme(
      <TagGlyphList tags={['ops']} initialMap={new Map([['ops', 'O']])} onTagClick={onTagClick} />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Tag ops' }), { shiftKey: true });
    const evt = onTagClick.mock.calls[0][1] as React.MouseEvent;
    expect(evt.shiftKey).toBe(true);
  });

  it('shows a +N overflow pill when tags exceed the cap', () => {
    const tags = Array.from({ length: TAG_GLYPH_MAX + 2 }, (_, i) => `tag${i + 1}`);
    const initialMap = new Map(tags.map((t) => [t, t[3].toUpperCase()]));
    renderWithTheme(<TagGlyphList tags={tags} initialMap={initialMap} onTagClick={() => {}} />);
    // First MAX glyphs render; tags after MAX do NOT render as glyphs.
    expect(screen.getByRole('button', { name: 'Tag tag1' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: `Tag tag${TAG_GLYPH_MAX + 1}` })).toBeNull();
    // The overflow pill is present with the right count.
    expect(screen.getByRole('button', { name: /2 more tags/ })).toBeTruthy();
  });

  it('does not show the overflow pill when tags fit', () => {
    renderWithTheme(
      <TagGlyphList tags={['a']} initialMap={new Map([['a', 'A']])} onTagClick={() => {}} />,
    );
    expect(screen.queryByRole('button', { name: /more tag/ })).toBeNull();
  });

  it('falls back to first letter when no initial map entry exists', () => {
    renderWithTheme(
      <TagGlyphList tags={['orphan']} initialMap={new Map()} onTagClick={() => {}} />,
    );
    expect(screen.getByRole('button', { name: 'Tag orphan' }).textContent).toBe('O');
  });
});
