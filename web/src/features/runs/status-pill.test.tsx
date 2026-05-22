import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { StatusPill } from './status-pill';

describe('StatusPill', () => {
  it('renders the status label for each status', () => {
    const { rerender } = render(<StatusPill status="running" />);
    expect(screen.getByText('running')).toBeTruthy();
    rerender(<StatusPill status="ok" />);
    expect(screen.getByText('ok')).toBeTruthy();
    rerender(<StatusPill status="error" />);
    expect(screen.getByText('error')).toBeTruthy();
    rerender(<StatusPill status="timeout" />);
    expect(screen.getByText('timeout')).toBeTruthy();
  });

  it('exposes the status via a data attribute', () => {
    const { container } = render(<StatusPill status="timeout" />);
    expect(container.querySelector('[data-status="timeout"]')).toBeTruthy();
  });
});
