/**
 * Tri-state checkbox indicator used by <BulkTagEditor>.
 *
 * Filled = tag will be (or already is) on all selected tasks after apply.
 * Half-bar = partial presence. Empty = absent. When the row is staged, the
 * box reflects the *post-apply* state for the current mode — so toggling a
 * tag in Remove mode flips a "filled" indicator back to "empty", giving the
 * user a live preview without having to mentally diff the chip strip.
 *
 * Lifted into its own file in the Phase 6 follow-up to keep
 * bulk-tag-editor.tsx focused on flow/state.
 */

import { CheckIcon, MinusIcon } from 'lucide-react';
import { cn } from '@/lib/utils';

export type TriStateMode = 'add' | 'remove' | 'set';
export type TriStatePresence = 'all' | 'some' | 'none';

export interface TriStateIndicatorProps {
  mode: TriStateMode;
  presence: TriStatePresence;
  staged: boolean;
}

export function TriStateIndicator({ mode, presence, staged }: TriStateIndicatorProps) {
  // Compute the *displayed* state — what the checkbox should show given the
  // current mode + staged flag.
  let display: 'filled' | 'half' | 'empty';
  if (mode === 'set') {
    display = staged ? 'filled' : 'empty';
  } else if (mode === 'add') {
    display = staged
      ? 'filled'
      : presence === 'all'
        ? 'filled'
        : presence === 'some'
          ? 'half'
          : 'empty';
  } else {
    // remove: staged means "will be removed → ends up empty"
    display = staged
      ? 'empty'
      : presence === 'all'
        ? 'filled'
        : presence === 'some'
          ? 'half'
          : 'empty';
  }
  return (
    <span
      aria-hidden
      data-slot="bulk-tag-editor-tristate"
      data-display={display}
      className={cn(
        'flex size-4 shrink-0 items-center justify-center rounded-[4px] border',
        display === 'empty' ? 'border-input' : 'border-primary bg-primary text-primary-foreground',
      )}
    >
      {display === 'filled' && <CheckIcon className="size-3" />}
      {display === 'half' && <MinusIcon className="size-3" />}
    </span>
  );
}
