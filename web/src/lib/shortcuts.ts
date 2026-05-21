import { useEffect } from 'react';
import type { router as Router } from '@/router';

type AppRouter = typeof Router;

const LEADER_TIMEOUT_MS = 1000;

const NAV_KEYS: Record<string, string> = {
  t: '/tasks',
  s: '/stage',
  c: '/scripts',
  g: '/tags',
  r: '/runs',
};

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
  if (target.isContentEditable) return true;
  return false;
}

function focusSearchInput(): boolean {
  const el = document.querySelector<HTMLInputElement>('[data-search-input]');
  if (el) {
    el.focus();
    el.select?.();
    return true;
  }
  return false;
}

export function useGlobalShortcuts(router: AppRouter) {
  useEffect(() => {
    let leader: 'g' | null = null;
    let leaderTimer: ReturnType<typeof setTimeout> | null = null;

    const clearLeader = () => {
      leader = null;
      if (leaderTimer) {
        clearTimeout(leaderTimer);
        leaderTimer = null;
      }
    };

    const startLeader = () => {
      leader = 'g';
      if (leaderTimer) clearTimeout(leaderTimer);
      leaderTimer = setTimeout(() => {
        leader = null;
        leaderTimer = null;
      }, LEADER_TIMEOUT_MS);
    };

    const handler = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      if (isEditableTarget(event.target)) return;

      const key = event.key;

      if (leader === 'g') {
        const target = NAV_KEYS[key];
        if (target) {
          event.preventDefault();
          clearLeader();
          void router.navigate({ to: target });
          return;
        }
        // Any other key cancels the leader chord.
        clearLeader();
        return;
      }

      if (key === 'g') {
        // Start leader chord (don't preventDefault yet — single 'g' alone does nothing).
        startLeader();
        return;
      }

      if (key === '?') {
        event.preventDefault();
        window.dispatchEvent(new CustomEvent('tt:toggle-cheatsheet'));
        return;
      }

      if (key === '/') {
        event.preventDefault();
        if (!focusSearchInput()) {
          void router.navigate({ to: '/tasks' }).then(() => {
            // Try again on the next tick after the page mounts.
            setTimeout(focusSearchInput, 0);
          });
        }
        return;
      }

      if (key === 'n') {
        event.preventDefault();
        window.dispatchEvent(new CustomEvent('tt:new-task'));
        return;
      }
    };

    window.addEventListener('keydown', handler);
    return () => {
      window.removeEventListener('keydown', handler);
      if (leaderTimer) clearTimeout(leaderTimer);
    };
  }, [router]);
}
