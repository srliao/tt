/**
 * Unit tests for <TagChip>.
 *
 * Wraps in <ThemeProvider> because the component reads `resolvedTheme` to
 * pick a light/dark palette. The two variants under test:
 *
 *   - normal: hash-derived color, `data-variant` reflects solid|outline.
 *   - untagged (name === UNTAGGED_TOKEN): italic muted label "Untagged",
 *     dashed border, dashed swatch, no hash-derived color.
 */

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ThemeProvider } from '@/components/theme-provider';
import { TagChip } from '@/components/ui/tag-chip';

function renderWithTheme(node: React.ReactNode) {
  return render(<ThemeProvider>{node}</ThemeProvider>);
}

describe('<TagChip>', () => {
  it('renders the tag name in the color-hashed variant', () => {
    renderWithTheme(<TagChip name="work" />);
    const chip = screen.getByText('work').closest('[data-slot="tag-chip"]');
    expect(chip).not.toBeNull();
    expect(chip?.getAttribute('data-variant')).toBe('solid');
    // The normal variant must NOT pick up the dashed-border class.
    expect(chip?.className).not.toMatch(/border-dashed/);
    expect(chip?.className).not.toMatch(/italic/);
  });

  it('respects the outline variant on the data attribute', () => {
    renderWithTheme(<TagChip name="work" variant="outline" />);
    const chip = screen.getByText('work').closest('[data-slot="tag-chip"]');
    expect(chip?.getAttribute('data-variant')).toBe('outline');
  });

  it('fires onClick when the chip is a button', () => {
    const onClick = vi.fn();
    renderWithTheme(<TagChip name="work" onClick={onClick} />);
    const btn = screen.getByRole('button', { name: 'Tag work' });
    fireEvent.click(btn);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('renders the X button when onRemove is provided', () => {
    const onRemove = vi.fn();
    renderWithTheme(<TagChip name="work" onRemove={onRemove} />);
    const removeBtn = screen.getByRole('button', { name: 'Remove work' });
    fireEvent.click(removeBtn);
    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  describe('untagged variant', () => {
    it('shows the visible label "Untagged" (not the @untagged sentinel)', () => {
      renderWithTheme(<TagChip name="@untagged" />);
      // Visible label is the user-facing string.
      expect(screen.getByText('Untagged')).toBeTruthy();
      expect(screen.queryByText('@untagged')).toBeNull();
    });

    it('marks the chip with data-variant="untagged" and the dashed/italic classes', () => {
      renderWithTheme(<TagChip name="@untagged" />);
      const chip = screen.getByText('Untagged').closest('[data-slot="tag-chip"]');
      expect(chip).not.toBeNull();
      expect(chip?.getAttribute('data-variant')).toBe('untagged');
      // Class signals of the variant: dashed border + italic muted text.
      expect(chip?.className).toMatch(/border-dashed/);
      expect(chip?.className).toMatch(/italic/);
      expect(chip?.className).toMatch(/text-muted-foreground/);
    });

    it('does not paint a hash-derived background color on the chip', () => {
      renderWithTheme(<TagChip name="@untagged" />);
      const chip = screen
        .getByText('Untagged')
        .closest('[data-slot="tag-chip"]') as HTMLElement | null;
      expect(chip).not.toBeNull();
      // Inline style.backgroundColor is unset for the untagged variant.
      expect(chip?.style.backgroundColor).toBe('');
    });

    it('forwards onClick and onRemove handlers like a normal chip', () => {
      const onClick = vi.fn();
      const onRemove = vi.fn();
      renderWithTheme(<TagChip name="@untagged" onClick={onClick} onRemove={onRemove} />);
      const btn = screen.getByRole('button', { name: 'Tag Untagged' });
      fireEvent.click(btn);
      expect(onClick).toHaveBeenCalledTimes(1);
      // Remove button labels use the friendly name, not the sentinel.
      const removeBtn = screen.getByRole('button', { name: 'Remove Untagged' });
      fireEvent.click(removeBtn);
      expect(onRemove).toHaveBeenCalledTimes(1);
      // The outer click handler should NOT fire when the X is clicked
      // (the X stops propagation).
      expect(onClick).toHaveBeenCalledTimes(1);
    });
  });
});
