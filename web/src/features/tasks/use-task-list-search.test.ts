import { describe, expect, it } from 'vitest';
import { applyQuickFilter, hasActiveFilters, taskSearchSchema } from './use-task-list-search';

describe('taskSearchSchema', () => {
  it('parses a fully-populated URL search object', () => {
    const parsed = taskSearchSchema.parse({
      states: ['not_done', 'done'],
      tags: ['work'],
      due: 'today',
      q: 'milk',
      sort: 'priority',
      asc: true,
      quick: 'overdue',
    });
    expect(parsed.states).toEqual(['not_done', 'done']);
    expect(parsed.quick).toBe('overdue');
  });

  it('treats empty input as defaults', () => {
    expect(taskSearchSchema.parse({})).toEqual({});
  });

  it('rejects unknown enum values', () => {
    expect(() => taskSearchSchema.parse({ sort: 'completed_at' as unknown as never })).toThrow();
  });
});

describe('applyQuickFilter', () => {
  it('returns the user filter when no quick preset is set', () => {
    expect(applyQuickFilter({ states: ['done'] })).toEqual({
      states: ['done'],
      tags: undefined,
      tagMode: undefined,
      due: undefined,
      q: undefined,
      sort: undefined,
      asc: undefined,
    });
  });

  it('defaults to not_done when no states and no quick preset are set', () => {
    expect(applyQuickFilter({})).toEqual({
      states: ['not_done'],
      tags: undefined,
      tagMode: undefined,
      due: undefined,
      q: undefined,
      sort: undefined,
      asc: undefined,
    });
  });

  it('sends tag_mode=any whenever tags are non-empty and no mode was set', () => {
    const out = applyQuickFilter({ tags: ['work'] });
    expect(out.tags).toEqual(['work']);
    expect(out.tagMode).toBe('any');
  });

  it('passes through an explicit tagMode=all', () => {
    const out = applyQuickFilter({ tags: ['work', 'urgent'], tagMode: 'all' });
    expect(out.tagMode).toBe('all');
  });

  it('omits tag_mode when no tags are selected', () => {
    expect(applyQuickFilter({ tagMode: 'all' }).tagMode).toBeUndefined();
  });

  it('expands the "overdue" preset', () => {
    expect(applyQuickFilter({ quick: 'overdue' })).toMatchObject({
      states: ['not_done'],
      due: 'overdue',
    });
  });

  it('expands "due-today"', () => {
    expect(applyQuickFilter({ quick: 'due-today' })).toMatchObject({
      states: ['not_done'],
      due: 'today',
    });
  });

  it('expands "recently-completed" with descending created_at', () => {
    expect(applyQuickFilter({ quick: 'recently-completed' })).toMatchObject({
      states: ['done'],
      sort: 'created_at',
      asc: false,
    });
  });

  it('expands "cancelled"', () => {
    expect(applyQuickFilter({ quick: 'cancelled' })).toMatchObject({
      states: ['cancelled'],
    });
  });

  it('lets explicit URL params override the preset states', () => {
    const out = applyQuickFilter({ quick: 'overdue', states: ['done'] });
    expect(out.states).toEqual(['done']);
    expect(out.due).toBe('overdue');
  });
});

describe('hasActiveFilters', () => {
  it('returns false for an empty search', () => {
    expect(hasActiveFilters({})).toBe(false);
  });

  it('returns true when any axis is set', () => {
    expect(hasActiveFilters({ q: 'x' })).toBe(true);
    expect(hasActiveFilters({ quick: 'overdue' })).toBe(true);
    expect(hasActiveFilters({ tags: ['work'] })).toBe(true);
    expect(hasActiveFilters({ states: ['done'] })).toBe(true);
  });
});
