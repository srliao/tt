import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { SoftCapHint, STAGE_CAP_DISMISSED_KEY } from './soft-cap-hint';

describe('SoftCapHint', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });
  afterEach(() => {
    sessionStorage.clear();
  });

  it('does not render at the soft cap', () => {
    render(<SoftCapHint count={7} />);
    expect(screen.queryByTestId('stage-soft-cap-hint')).toBeNull();
  });

  it('renders when count exceeds the cap', () => {
    render(<SoftCapHint count={8} />);
    expect(screen.getByTestId('stage-soft-cap-hint')).toBeTruthy();
  });

  it('dismissal hides the hint and persists in sessionStorage', () => {
    render(<SoftCapHint count={9} />);
    const dismiss = screen.getByRole('button', { name: 'Dismiss stage soft-cap hint' });
    act(() => {
      dismiss.click();
    });
    expect(screen.queryByTestId('stage-soft-cap-hint')).toBeNull();
    expect(sessionStorage.getItem(STAGE_CAP_DISMISSED_KEY)).toBe('1');
  });

  it('respects existing dismissal in sessionStorage', () => {
    sessionStorage.setItem(STAGE_CAP_DISMISSED_KEY, '1');
    render(<SoftCapHint count={10} />);
    expect(screen.queryByTestId('stage-soft-cap-hint')).toBeNull();
  });

  it('clears the dismissal flag when the count drops back to the cap', () => {
    sessionStorage.setItem(STAGE_CAP_DISMISSED_KEY, '1');
    const { rerender } = render(<SoftCapHint count={9} />);
    expect(screen.queryByTestId('stage-soft-cap-hint')).toBeNull();
    // Count drops back into the cap — the dismissal should be reset so the
    // next breach re-shows the hint.
    rerender(<SoftCapHint count={5} />);
    expect(sessionStorage.getItem(STAGE_CAP_DISMISSED_KEY)).toBeNull();
    rerender(<SoftCapHint count={8} />);
    expect(screen.getByTestId('stage-soft-cap-hint')).toBeTruthy();
  });
});
