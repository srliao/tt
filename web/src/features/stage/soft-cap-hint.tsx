/**
 * Dismissible hint banner shown above the stage list when the staged count
 * exceeds the spec's "focused" soft cap of 7.
 *
 * The dismissal is per-session (`sessionStorage["tt.stage-cap-dismissed"]`)
 * so closing the tab clears it. We deliberately don't use `localStorage`:
 * the cap is meant to be a gentle reminder, not a one-time onboarding tip.
 */

import { XIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';

/** Threshold above which the hint becomes visible. */
export const STAGE_SOFT_CAP = 7;

/** sessionStorage key used to suppress the hint within a session. */
export const STAGE_CAP_DISMISSED_KEY = 'tt.stage-cap-dismissed';

export interface SoftCapHintProps {
  count: number;
}

function readDismissed(): boolean {
  try {
    return sessionStorage.getItem(STAGE_CAP_DISMISSED_KEY) === '1';
  } catch {
    return false;
  }
}

export function SoftCapHint({ count }: SoftCapHintProps) {
  const [dismissed, setDismissed] = useState<boolean>(() => readDismissed());

  // If the count drops back to or below the cap, reset our dismissed flag so
  // the hint reappears the *next* time the user crosses the threshold.
  useEffect(() => {
    if (count <= STAGE_SOFT_CAP && dismissed) {
      try {
        sessionStorage.removeItem(STAGE_CAP_DISMISSED_KEY);
      } catch {
        // ignored
      }
      setDismissed(false);
    }
  }, [count, dismissed]);

  if (count <= STAGE_SOFT_CAP) return null;
  if (dismissed) return null;

  const onDismiss = () => {
    try {
      sessionStorage.setItem(STAGE_CAP_DISMISSED_KEY, '1');
    } catch {
      // ignored — degrade gracefully if sessionStorage is unavailable
    }
    setDismissed(true);
  };

  return (
    <div
      role="status"
      data-testid="stage-soft-cap-hint"
      className="flex items-start gap-2 rounded-md border border-amber-300/60 bg-amber-100/40 px-3 py-2 text-xs text-amber-900 dark:border-amber-700/50 dark:bg-amber-900/20 dark:text-amber-200"
    >
      <p className="flex-1">
        Focused stages stay small — consider clearing finished items or unstaging anything that can
        wait.
      </p>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="Dismiss stage soft-cap hint"
        onClick={onDismiss}
      >
        <XIcon className="size-3" aria-hidden="true" />
      </Button>
    </div>
  );
}
