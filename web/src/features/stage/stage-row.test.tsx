import { act, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Task } from '@/types/task';
import { StageRow, toggleDone } from './stage-row';

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

describe('toggleDone', () => {
  it('toggles between done and not_done', () => {
    expect(toggleDone('not_done')).toBe('done');
    expect(toggleDone('done')).toBe('not_done');
  });

  it('treats cancelled as not_done (clicking the radio marks it done)', () => {
    expect(toggleDone('cancelled')).toBe('done');
  });
});

describe('StageRow', () => {
  it('applies line-through to a done row', () => {
    render(
      <StageRow
        task={task({ id: 1, title: 'Buy milk', state: 'done' })}
        onEdit={() => {}}
        onToggleDone={() => {}}
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
        onToggleDone={() => {}}
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
        onToggleDone={() => {}}
        onUnstage={() => {}}
      />,
    );
    const title = screen.getByRole('button', { name: 'Open task' });
    expect(title).not.toHaveClass('line-through');
  });

  it('invokes onToggleDone when the done radio is clicked', () => {
    const onToggleDone = vi.fn();
    render(
      <StageRow
        task={task({ id: 5, title: 'Foo' })}
        onEdit={() => {}}
        onToggleDone={onToggleDone}
        onUnstage={() => {}}
      />,
    );
    act(() => {
      screen.getByRole('button', { name: /Mark Foo as done/ }).click();
    });
    expect(onToggleDone).toHaveBeenCalledTimes(1);
  });

  it('invokes onUnstage when the bookmark is clicked', () => {
    const onUnstage = vi.fn();
    render(
      <StageRow
        task={task({ id: 5, title: 'Foo' })}
        onEdit={() => {}}
        onToggleDone={() => {}}
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
        onToggleDone={() => {}}
        onUnstage={() => {}}
      />,
    );
    act(() => {
      screen.getByRole('button', { name: 'Foo' }).click();
    });
    expect(onEdit).toHaveBeenCalledTimes(1);
  });
});
