import { describe, expect, it } from 'vitest';
import { tagColor, tagColorDark } from './tag-color';

describe('tagColor()', () => {
  it('is stable for the same name', () => {
    const a = tagColor('foo');
    const b = tagColor('foo');
    expect(a).toEqual(b);
  });

  it('is case-insensitive', () => {
    expect(tagColor('Foo')).toEqual(tagColor('foo'));
    expect(tagColor('WORK')).toEqual(tagColor('work'));
    expect(tagColor('MixedCase')).toEqual(tagColor('mixedcase'));
  });

  it('returns the three expected slots', () => {
    const c = tagColor('home');
    expect(c).toHaveProperty('bg');
    expect(c).toHaveProperty('fg');
    expect(c).toHaveProperty('dot');
    expect(c.bg).toMatch(/^oklch\(/);
    expect(c.fg).toMatch(/^oklch\(/);
    expect(c.dot).toMatch(/^oklch\(/);
  });

  it('produces different colors for sufficiently different names', () => {
    // The palette is only 12 hues so collisions are possible; pick a handful
    // of well-separated inputs.
    const names = ['alpha', 'beta', 'gamma', 'delta', 'omega'];
    const hues = new Set(names.map((n) => tagColor(n).dot));
    // Expect at least 3 distinct hues out of 5 — generous to allow collisions.
    expect(hues.size).toBeGreaterThanOrEqual(3);
  });
});

describe('tagColor() with explicit hue', () => {
  it('uses the supplied hue instead of hashing the name', () => {
    // Two unrelated names get the same color when the same hue is forced,
    // proving the hash is bypassed. Backend assigns hues via least-used-hue
    // so the frontend must honor them verbatim — otherwise we lose the
    // collision-free guarantee.
    const a = tagColor('foo', 90);
    const b = tagColor('bar', 90);
    expect(a).toEqual(b);
  });

  it('snaps unknown hues into the oklch() output verbatim', () => {
    const c = tagColor('whatever', 210);
    expect(c.bg).toContain('210');
    expect(c.fg).toContain('210');
    expect(c.dot).toContain('210');
  });

  it('falls back to the name hash when hue is undefined', () => {
    expect(tagColor('foo', undefined)).toEqual(tagColor('foo'));
  });
});

describe('tagColorDark()', () => {
  it('is stable and case-insensitive', () => {
    expect(tagColorDark('Foo')).toEqual(tagColorDark('foo'));
  });

  it('shares hue with the light variant', () => {
    // Both variants pull the hue from the same palette index, so the trailing
    // hue number inside the oklch(...) string must match.
    const hueOf = (s: string) => s.match(/(\d+(?:\.\d+)?)\)$/)?.[1];
    expect(hueOf(tagColorDark('foo').bg)).toBe(hueOf(tagColor('foo').bg));
  });

  it('uses different lightness than the light variant', () => {
    expect(tagColorDark('foo').bg).not.toEqual(tagColor('foo').bg);
  });
});
