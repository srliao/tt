import { describe, expect, it } from 'vitest';
import type { Task } from '@/types/task';
import {
  applyQuickFilter,
  clickModeFromEvent,
  computeAllMatchingIds,
  hasActiveFilters,
  isStateRestricted,
  matchesFilter,
  parseTagFilter,
  serializeTagFilter,
  type TagFilter,
  taskSearchSchema,
  UNTAGGED_TOKEN,
} from './use-task-list-search';

describe('taskSearchSchema', () => {
  it('parses a fully-populated URL search object', () => {
    const parsed = taskSearchSchema.parse({
      states: ['not_done', 'done'],
      tag_filter: 'any:work',
      due: 'today',
      q: 'milk',
      sort: 'priority',
      asc: true,
      quick: 'overdue',
    });
    expect(parsed.states).toEqual(['not_done', 'done']);
    expect(parsed.quick).toBe('overdue');
    expect(parsed.tag_filter).toEqual({ mode: 'any', tags: ['work'] });
  });

  it('treats empty input as defaults', () => {
    expect(taskSearchSchema.parse({})).toEqual({});
  });

  it('rejects unknown enum values', () => {
    expect(() => taskSearchSchema.parse({ sort: 'completed_at' as unknown as never })).toThrow();
  });

  it('does not surface the legacy tags or tagMode fields', () => {
    const parsed = taskSearchSchema.parse({
      tag_filter: 'any:work',
    });
    expect((parsed as Record<string, unknown>).tags).toBeUndefined();
    expect((parsed as Record<string, unknown>).tagMode).toBeUndefined();
  });

  it('parses tag_filter from a URL string into a structured object', () => {
    const parsed = taskSearchSchema.parse({ tag_filter: 'all:work,urgent' });
    expect(parsed.tag_filter).toEqual({ mode: 'all', tags: ['work', 'urgent'] });
  });

  it('drops a malformed tag_filter silently', () => {
    const parsed = taskSearchSchema.parse({ tag_filter: 'garbage' });
    expect(parsed.tag_filter).toBeUndefined();
  });

  it('leaves tag_filter undefined when the param is absent', () => {
    const parsed = taskSearchSchema.parse({});
    expect(parsed.tag_filter).toBeUndefined();
  });

  it('mirrors the route /tasks?tag_filter=any:work,errand contract', () => {
    // Simulate what TanStack Router hands `validateSearch`: a string-valued
    // search-param record. The route in `routes/tasks.tsx` calls
    // `taskSearchSchema.parse(search)` directly so this assertion stands
    // in for a full route-validation round-trip.
    const parsed = taskSearchSchema.parse({ tag_filter: 'any:work,errand' });
    expect(parsed.tag_filter).toEqual({ mode: 'any', tags: ['work', 'errand'] });
  });

  it('mirrors the empty /tasks route contract (no tag_filter)', () => {
    const parsed = taskSearchSchema.parse({});
    expect(parsed.tag_filter).toBeUndefined();
  });
});

describe('parseTagFilter', () => {
  it('parses a simple any-mode filter', () => {
    expect(parseTagFilter('any:work,errand')).toEqual({
      mode: 'any',
      tags: ['work', 'errand'],
    });
  });

  it('parses an all-mode filter', () => {
    expect(parseTagFilter('all:work,urgent')).toEqual({
      mode: 'all',
      tags: ['work', 'urgent'],
    });
  });

  it('parses a single-tag filter', () => {
    expect(parseTagFilter('any:work')).toEqual({ mode: 'any', tags: ['work'] });
  });

  it('preserves the @untagged sentinel', () => {
    expect(parseTagFilter('any:@untagged')).toEqual({
      mode: 'any',
      tags: ['@untagged'],
    });
  });

  it('preserves a mixed @untagged + real tag set', () => {
    expect(parseTagFilter('all:work,@untagged')).toEqual({
      mode: 'all',
      tags: ['work', '@untagged'],
    });
  });

  it('trims whitespace and drops empty segments', () => {
    expect(parseTagFilter('any: work , , errand ')).toEqual({
      mode: 'any',
      tags: ['work', 'errand'],
    });
  });

  it('returns undefined for input with no colon', () => {
    expect(parseTagFilter('garbage')).toBeUndefined();
  });

  it('returns undefined for an unknown mode', () => {
    expect(parseTagFilter('maybe:work')).toBeUndefined();
  });

  it('returns undefined for an empty tag list', () => {
    expect(parseTagFilter('any:')).toBeUndefined();
    expect(parseTagFilter('any:,,')).toBeUndefined();
  });
});

describe('serializeTagFilter', () => {
  it('joins tags with commas after the mode', () => {
    expect(serializeTagFilter({ mode: 'any', tags: ['work', 'errand'] })).toBe('any:work,errand');
  });

  it('serialises a single tag', () => {
    expect(serializeTagFilter({ mode: 'all', tags: ['work'] })).toBe('all:work');
  });

  it('serialises the @untagged sentinel', () => {
    expect(serializeTagFilter({ mode: 'any', tags: [UNTAGGED_TOKEN] })).toBe('any:@untagged');
  });

  it('returns undefined for an empty tags array', () => {
    expect(serializeTagFilter({ mode: 'any', tags: [] })).toBeUndefined();
  });

  it('returns undefined for an undefined input', () => {
    expect(serializeTagFilter(undefined)).toBeUndefined();
  });
});

describe('parseTagFilter / serializeTagFilter round-trip', () => {
  const cases: TagFilter[] = [
    { mode: 'any', tags: ['work'] },
    { mode: 'all', tags: ['work', 'urgent'] },
    { mode: 'any', tags: [UNTAGGED_TOKEN] },
    { mode: 'all', tags: ['work', UNTAGGED_TOKEN] },
  ];
  for (const f of cases) {
    it(`round-trips ${JSON.stringify(f)}`, () => {
      const s = serializeTagFilter(f);
      expect(s).toBeDefined();
      expect(parseTagFilter(s as string)).toEqual(f);
    });
  }
});

describe('applyQuickFilter', () => {
  it('returns the user filter when no quick preset is set', () => {
    expect(applyQuickFilter({ states: ['done'] })).toEqual({
      states: ['done'],
      tag_filter: undefined,
      tagsExclude: undefined,
      due: undefined,
      q: undefined,
      sort: undefined,
      asc: undefined,
    });
  });

  it('defaults to not_done when no states and no quick preset are set', () => {
    expect(applyQuickFilter({})).toEqual({
      states: ['not_done'],
      tag_filter: undefined,
      tagsExclude: undefined,
      due: undefined,
      q: undefined,
      sort: undefined,
      asc: undefined,
    });
  });

  it('forwards an any-mode tag_filter as-is', () => {
    const out = applyQuickFilter({ tag_filter: { mode: 'any', tags: ['work'] } });
    expect(out.tag_filter).toEqual({ mode: 'any', tags: ['work'] });
  });

  it('forwards an all-mode tag_filter as-is', () => {
    const out = applyQuickFilter({ tag_filter: { mode: 'all', tags: ['work', 'urgent'] } });
    expect(out.tag_filter).toEqual({ mode: 'all', tags: ['work', 'urgent'] });
  });

  it('passes tag_filter through alongside a quick preset', () => {
    const out = applyQuickFilter({
      quick: 'overdue',
      tag_filter: { mode: 'any', tags: ['work'] },
    });
    expect(out.tag_filter).toEqual({ mode: 'any', tags: ['work'] });
    expect(out.due).toBe('overdue');
    expect(out.states).toEqual(['not_done']);
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

describe('clickModeFromEvent', () => {
  function mkEvent(opts: { altKey?: boolean; shiftKey?: boolean } = {}) {
    return {
      altKey: !!opts.altKey,
      shiftKey: !!opts.shiftKey,
    } as React.MouseEvent;
  }

  it('returns "replace" for a bare click', () => {
    expect(clickModeFromEvent(mkEvent())).toBe('replace');
  });

  it('returns "add" for shift-click', () => {
    expect(clickModeFromEvent(mkEvent({ shiftKey: true }))).toBe('add');
  });

  it('returns "exclude" for alt-click', () => {
    expect(clickModeFromEvent(mkEvent({ altKey: true }))).toBe('exclude');
  });

  it('prefers exclude when both alt and shift are pressed', () => {
    expect(clickModeFromEvent(mkEvent({ altKey: true, shiftKey: true }))).toBe('exclude');
  });
});

describe('taskSearchSchema (tagsExclude)', () => {
  it('parses tagsExclude as a string array', () => {
    const parsed = taskSearchSchema.parse({ tagsExclude: ['noise', 'archived'] });
    expect(parsed.tagsExclude).toEqual(['noise', 'archived']);
  });
});

describe('hasActiveFilters', () => {
  it('returns false for an empty search', () => {
    expect(hasActiveFilters({})).toBe(false);
  });

  it('returns true when any axis is set', () => {
    expect(hasActiveFilters({ q: 'x' })).toBe(true);
    expect(hasActiveFilters({ quick: 'overdue' })).toBe(true);
    expect(hasActiveFilters({ tag_filter: { mode: 'any', tags: ['work'] } })).toBe(true);
    expect(hasActiveFilters({ tagsExclude: ['noise'] })).toBe(true);
    expect(hasActiveFilters({ states: ['done'] })).toBe(true);
  });
});

describe('isStateRestricted', () => {
  it('returns true when no states are set (default view = not_done only)', () => {
    expect(isStateRestricted({})).toBe(true);
  });

  it('returns true when states is empty (treated as default)', () => {
    expect(isStateRestricted({ states: [] })).toBe(true);
  });

  it('returns false when all three states are explicitly selected', () => {
    expect(isStateRestricted({ states: ['not_done', 'done', 'cancelled'] })).toBe(false);
  });

  it('returns true when only a strict subset of states is selected', () => {
    expect(isStateRestricted({ states: ['not_done'] })).toBe(true);
    expect(isStateRestricted({ states: ['done'] })).toBe(true);
    expect(isStateRestricted({ states: ['not_done', 'done'] })).toBe(true);
  });
});

describe('applyQuickFilter (tagsExclude)', () => {
  it('propagates tagsExclude through to the params', () => {
    const out = applyQuickFilter({ tagsExclude: ['noise'] });
    expect(out.tagsExclude).toEqual(['noise']);
  });

  it('omits tagsExclude when empty', () => {
    expect(applyQuickFilter({ tagsExclude: [] }).tagsExclude).toBeUndefined();
  });

  it('preserves tagsExclude alongside a quick preset', () => {
    const out = applyQuickFilter({ quick: 'overdue', tagsExclude: ['noise'] });
    expect(out.tagsExclude).toEqual(['noise']);
    expect(out.due).toBe('overdue');
  });
});

function mkTask(partial: Partial<Task> & { id: number }): Task {
  return {
    title: '',
    notes: '',
    state: 'not_done',
    due_date: null,
    priority: 0,
    staged_order: null,
    spawned_by_script_id: null,
    created_at: '2026-05-01T00:00:00Z',
    completed_at: null,
    cancelled_at: null,
    updated_at: '2026-05-01T00:00:00Z',
    tags: [],
    ...partial,
  };
}

function ymd(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

describe('matchesFilter — states', () => {
  it('passes when filter.states is unset or empty', () => {
    expect(matchesFilter(mkTask({ id: 1, state: 'not_done' }), {})).toBe(true);
    expect(matchesFilter(mkTask({ id: 1, state: 'done' }), { states: [] })).toBe(true);
  });

  it('keeps tasks whose state is listed', () => {
    expect(matchesFilter(mkTask({ id: 1, state: 'done' }), { states: ['done', 'cancelled'] })).toBe(
      true,
    );
  });

  it('drops tasks whose state is not listed', () => {
    expect(matchesFilter(mkTask({ id: 1, state: 'not_done' }), { states: ['done'] })).toBe(false);
  });
});

describe('matchesFilter — tag_filter mode=any', () => {
  it('matches when at least one filter tag is on the task', () => {
    const t = mkTask({ id: 1, tags: ['infra', 'urgent'] });
    expect(matchesFilter(t, { tag_filter: { mode: 'any', tags: ['infra'] } })).toBe(true);
    expect(matchesFilter(t, { tag_filter: { mode: 'any', tags: ['infra', 'cleanup'] } })).toBe(
      true,
    );
    expect(matchesFilter(t, { tag_filter: { mode: 'any', tags: ['cleanup'] } })).toBe(false);
  });

  it('untagged-only matches a task with zero tags', () => {
    expect(
      matchesFilter(mkTask({ id: 1, tags: [] }), {
        tag_filter: { mode: 'any', tags: [UNTAGGED_TOKEN] },
      }),
    ).toBe(true);
    expect(
      matchesFilter(mkTask({ id: 1, tags: ['infra'] }), {
        tag_filter: { mode: 'any', tags: [UNTAGGED_TOKEN] },
      }),
    ).toBe(false);
  });

  it('untagged + real tags is a union (untagged OR tagged-with-any)', () => {
    // Tagged with one of the real tags → matches.
    expect(
      matchesFilter(mkTask({ id: 1, tags: ['work'] }), {
        tag_filter: { mode: 'any', tags: [UNTAGGED_TOKEN, 'work'] },
      }),
    ).toBe(true);
    // Untagged → also matches.
    expect(
      matchesFilter(mkTask({ id: 1, tags: [] }), {
        tag_filter: { mode: 'any', tags: [UNTAGGED_TOKEN, 'work'] },
      }),
    ).toBe(true);
    // Neither → drops.
    expect(
      matchesFilter(mkTask({ id: 1, tags: ['other'] }), {
        tag_filter: { mode: 'any', tags: [UNTAGGED_TOKEN, 'work'] },
      }),
    ).toBe(false);
  });
});

describe('matchesFilter — tag_filter mode=all', () => {
  it('requires every filter tag to appear', () => {
    const t = mkTask({ id: 1, tags: ['infra', 'urgent'] });
    expect(matchesFilter(t, { tag_filter: { mode: 'all', tags: ['infra', 'urgent'] } })).toBe(true);
    expect(matchesFilter(t, { tag_filter: { mode: 'all', tags: ['infra', 'missing'] } })).toBe(
      false,
    );
  });

  it('untagged-only matches a task with zero tags', () => {
    expect(
      matchesFilter(mkTask({ id: 1, tags: [] }), {
        tag_filter: { mode: 'all', tags: [UNTAGGED_TOKEN] },
      }),
    ).toBe(true);
  });

  it('untagged + real tags is unsatisfiable (no task is both)', () => {
    expect(
      matchesFilter(mkTask({ id: 1, tags: [] }), {
        tag_filter: { mode: 'all', tags: [UNTAGGED_TOKEN, 'work'] },
      }),
    ).toBe(false);
    expect(
      matchesFilter(mkTask({ id: 1, tags: ['work'] }), {
        tag_filter: { mode: 'all', tags: [UNTAGGED_TOKEN, 'work'] },
      }),
    ).toBe(false);
  });
});

describe('matchesFilter — tagsExclude', () => {
  it('drops the task when any excluded tag appears', () => {
    const t = mkTask({ id: 1, tags: ['infra', 'noise'] });
    expect(matchesFilter(t, { tagsExclude: ['noise'] })).toBe(false);
  });

  it('keeps the task when no excluded tag appears', () => {
    const t = mkTask({ id: 1, tags: ['infra'] });
    expect(matchesFilter(t, { tagsExclude: ['noise'] })).toBe(true);
  });
});

describe('matchesFilter — due', () => {
  const today = ymd(new Date());
  const yesterday = ymd(new Date(Date.now() - 24 * 60 * 60 * 1000));
  const inThreeDays = ymd(new Date(Date.now() + 3 * 24 * 60 * 60 * 1000));
  const inTenDays = ymd(new Date(Date.now() + 10 * 24 * 60 * 60 * 1000));

  it('overdue: due_date strictly before today AND state !== done', () => {
    expect(matchesFilter(mkTask({ id: 1, due_date: yesterday }), { due: 'overdue' })).toBe(true);
    // Same task but completed today → not overdue.
    expect(
      matchesFilter(mkTask({ id: 1, due_date: yesterday, state: 'done' }), { due: 'overdue' }),
    ).toBe(false);
    // Today is not overdue.
    expect(matchesFilter(mkTask({ id: 1, due_date: today }), { due: 'overdue' })).toBe(false);
    // No due_date is not overdue.
    expect(matchesFilter(mkTask({ id: 1, due_date: null }), { due: 'overdue' })).toBe(false);
  });

  it('today: due_date equals today', () => {
    expect(matchesFilter(mkTask({ id: 1, due_date: today }), { due: 'today' })).toBe(true);
    expect(matchesFilter(mkTask({ id: 1, due_date: yesterday }), { due: 'today' })).toBe(false);
  });

  it('this_week: due_date within today..today+7', () => {
    expect(matchesFilter(mkTask({ id: 1, due_date: inThreeDays }), { due: 'this_week' })).toBe(
      true,
    );
    expect(matchesFilter(mkTask({ id: 1, due_date: inTenDays }), { due: 'this_week' })).toBe(false);
    expect(matchesFilter(mkTask({ id: 1, due_date: yesterday }), { due: 'this_week' })).toBe(false);
  });

  it('none: matches only tasks with no due_date', () => {
    expect(matchesFilter(mkTask({ id: 1, due_date: null }), { due: 'none' })).toBe(true);
    expect(matchesFilter(mkTask({ id: 1, due_date: today }), { due: 'none' })).toBe(false);
  });
});

describe('matchesFilter — q', () => {
  it('matches substrings in the title case-insensitively', () => {
    const t = mkTask({ id: 1, title: 'Ship the docs', notes: '' });
    expect(matchesFilter(t, { q: 'ship' })).toBe(true);
    expect(matchesFilter(t, { q: 'DOCS' })).toBe(true);
    expect(matchesFilter(t, { q: 'nope' })).toBe(false);
  });

  it('matches substrings in notes case-insensitively', () => {
    const t = mkTask({ id: 1, title: 'A', notes: 'Followup: rotate keys' });
    expect(matchesFilter(t, { q: 'rotate' })).toBe(true);
    expect(matchesFilter(t, { q: 'ROTATE' })).toBe(true);
  });
});

describe('computeAllMatchingIds', () => {
  it('returns the ids of every task satisfying the filter', () => {
    const tasks = [
      mkTask({ id: 1, state: 'not_done', tags: ['infra'] }),
      mkTask({ id: 2, state: 'done', tags: ['infra'] }),
      mkTask({ id: 3, state: 'not_done', tags: ['other'] }),
    ];
    expect(
      computeAllMatchingIds(tasks, {
        states: ['not_done'],
        tag_filter: { mode: 'any', tags: ['infra'] },
      }),
    ).toEqual([1]);
  });

  it('returns every id when the filter is empty', () => {
    const tasks = [mkTask({ id: 1 }), mkTask({ id: 2 })];
    expect(computeAllMatchingIds(tasks, {})).toEqual([1, 2]);
  });
});
