import type { ReactNode } from 'react';

/**
 * AppLayout placeholder — fleshed out in Task 6 (top nav, theme toggle,
 * Stage badge, cheatsheet mount).
 */
export function AppLayout({ children }: { children: ReactNode }) {
  return <div className="min-h-screen bg-background text-foreground">{children}</div>;
}
