import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import type { ReactNode } from 'react';
import { Separator } from '@/components/ui/separator';
import { api } from '@/lib/api';
import { ShortcutCheatsheet } from './shortcut-cheatsheet';
import { ThemeProvider } from './theme-provider';
import { ThemeToggle } from './theme-toggle';

interface StageCountTask {
  staged_order: number | null;
  state: 'not_done' | 'done' | 'cancelled';
}

const NAV_ITEMS = [
  { to: '/stage', label: 'Stage' },
  { to: '/tasks', label: 'Tasks' },
  { to: '/scripts', label: 'Scripts' },
  { to: '/tags', label: 'Tags' },
  { to: '/runs', label: 'Runs' },
] as const;

function useStagedCount() {
  const { data } = useQuery<StageCountTask[]>({
    queryKey: ['tasks'],
    queryFn: () => api<StageCountTask[]>('/tasks'),
    retry: false,
  });
  return data?.filter((t) => t.staged_order !== null && t.state === 'not_done').length ?? 0;
}

function TopNav() {
  const stagedCount = useStagedCount();
  return (
    <header className="sticky top-0 z-30 border-b bg-background/95 backdrop-blur supports-backdrop-filter:bg-background/60">
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-4 px-4">
        <Link
          to="/stage"
          className="font-mono text-lg font-semibold tracking-tight"
          aria-label="tt home"
        >
          tt
        </Link>
        <Separator orientation="vertical" className="h-6" />
        <nav className="flex items-center gap-1 text-sm">
          {NAV_ITEMS.map((item) => (
            <Link
              key={item.to}
              to={item.to}
              className="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
              activeProps={{
                className:
                  'flex items-center gap-1.5 rounded-md px-3 py-1.5 bg-accent text-accent-foreground',
              }}
            >
              {item.label}
              {item.to === '/stage' && stagedCount > 0 && (
                <span
                  data-testid="stage-badge"
                  className="inline-flex min-w-[1.5em] items-center justify-center rounded-full bg-primary px-1.5 leading-none font-semibold text-primary-foreground"
                >
                  {stagedCount}
                </span>
              )}
            </Link>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
        </div>
      </div>
    </header>
  );
}

export function AppLayout({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <div className="min-h-screen bg-background text-foreground">
        <TopNav />
        <main className="mx-auto max-w-6xl">{children}</main>
        <ShortcutCheatsheet />
      </div>
    </ThemeProvider>
  );
}
