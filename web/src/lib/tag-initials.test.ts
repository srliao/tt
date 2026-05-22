import { describe, expect, it } from 'vitest';
import { buildInitialMap } from './tag-initials';

describe('buildInitialMap()', () => {
  it('returns an empty map for empty input', () => {
    const m = buildInitialMap([]);
    expect(m.size).toBe(0);
  });

  it('uses a single uppercase letter when there is no collision', () => {
    const m = buildInitialMap(['backend']);
    expect(m.get('backend')).toBe('B');
  });

  it('disambiguates with first two letters when only the first letter collides', () => {
    // Different second letters — two-letter form works.
    const m = buildInitialMap(['backend', 'build']);
    expect(m.get('backend')).toBe('Ba');
    expect(m.get('build')).toBe('Bu');
  });

  it('falls back to first-letter + index when second letters also collide', () => {
    const m = buildInitialMap(['backend', 'backups', 'backfill']);
    // All three share `ba`, so the two-letter disambiguation cannot separate
    // them — produce stable B1/B2/B3 in sorted order.
    const sorted = ['backend', 'backfill', 'backups'];
    sorted.forEach((name, i) => {
      expect(m.get(name)).toBe(`B${i + 1}`);
    });
  });

  it('keeps single-letter initials when groups do not collide', () => {
    const m = buildInitialMap(['ops', 'backend']);
    expect(m.get('ops')).toBe('O');
    expect(m.get('backend')).toBe('B');
  });

  it('is case-insensitive when grouping by first letter', () => {
    const m = buildInitialMap(['Backend', 'build']);
    expect(m.get('Backend')).toBe('Ba');
    expect(m.get('build')).toBe('Bu');
  });

  it('handles an empty-string entry without crashing', () => {
    const m = buildInitialMap(['']);
    expect(m.get('')).toBe('?');
  });
});
