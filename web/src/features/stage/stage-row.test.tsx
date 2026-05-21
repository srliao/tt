import { act, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { nextState, StageRow } from './stage-row';

function task(partial: Partial<Task> & { id: number; title: string }): Task {
  return {
    notes: '',
    state: 'not_done',
    due_date: null,
    priority: 0,
    staged_order: 1,
    spawned_by_script_id: null,
    created_at: '2026-05-01T00:00:00Z',
    completed_at: null,
    cancelled_at: null,
    updated_at: '2026-05-01T00:00:00Z',
    tags: [],
    ...partial,
  };
}

describe('nextState', () => {
  it('cycles not_done → done → cancelled → not_done', () => {
    expect(nextState('not_done')).toBe('done');
    expect(nextState('done')).toBe('cancelled');
    expect(nextState('cancelled')).toBe('not_done');
  });
});

describe('StageRow', () => {
  it('applies line-through to a done row', () => {
    render(
      <StageRow
        task={task({ id: 1, title: 'Buy milk', state: 'done' })}
        onEdit={() => {}}
        onCycleState={() => {}}
        onUnstage={() => {}}
      />,
    );
    const title = screen.getByRole('button', { name: 'Buy milk' });
    expect(title).toHaveClass('line-through');
  });

  it('applies line-through + de-emphasised classes to a cancelled row', () => {
    const { container } = render(
      <StageRow
        task={task({ id: 1, title: 'Skip', state: 'cancelled' })}
        onEdit={() => {}}
        onCycleState={() => {}}
        onUnstage={() => {}}
      />,
    );
    const title = screen.getByRole('button', { name: 'Skip' });
    expect(title).toHaveClass('line-through');
    const row = container.querySelector('[data-task-id="1"]');
    expect(row).toBeTruthy();
    expect(row?.className).toContain('opacity-60');
  });

  it('does not apply line-through to a not_done row', () => {
    render(
      <StageRow
        task={task({ id: 1, title: 'Open task' })}
        onEdit={() => {}}
        onCycleState={() => {}}
        onUnstage={() => {}}
      />,
    );
    const title = screen.getByRole('button', { name: 'Open task' });
    expect(title).not.toHaveClass('line-through');
  });

  it('invokes onCycleState when the state toggle is clicked', () => {
    const onCycleState = vi.fn();
    render(
      <StageRow
        task={task({ id: 5, title: 'Foo' })}
        onEdit={() => {}}
        onCycleState={onCycleState}
        onUnstage={() => {}}
      />,
    );
    act(() => {
      screen.getByRole('button', { name: /Cycle state for Foo/ }).click();
    });
    expect(onCycleState).toHaveBeenCalledTimes(1);
  });

  it('invokes onUnstage when the unstage button is clicked', () => {
    const onUnstage = vi.fn();
    render(
      <StageRow
        task={task({ id: 5, title: 'Foo' })}
        onEdit={() => {}}
        onCycleState={() => {}}
        onUnstage={onUnstage}
      />,
    );
    act(() => {
      screen.getByRole('button', { name: 'Unstage Foo' }).click();
    });
    expect(onUnstage).toHaveBeenCalledTimes(1);
  });

  it('invokes onEdit when the title is clicked', () => {
    const onEdit = vi.fn();
    render(
      <StageRow
        task={task({ id: 5, title: 'Foo' })}
        onEdit={onEdit}
        onCycleState={() => {}}
        onUnstage={() => {}}
      />,
    );
    act(() => {
      screen.getByRole('button', { name: 'Foo' }).click();
    });
    expect(onEdit).toHaveBeenCalledTimes(1);
  });
});
