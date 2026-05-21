import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import type { ReactNode } from 'react';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import { api } from '@/lib/api';
import { ShortcutCheatsheet } from './shortcut-cheatsheet';
import { ThemeProvider } from './theme-provider';
import { ThemeToggle } from './theme-toggle';

interface StageCountTask {
  staged_order: number | null;
}

const NAV_ITEMS = [
  { to: '/tasks', label: 'Tasks' },
  { to: '/stage', label: 'Stage' },
  { to: '/scripts', label: 'Scripts' },
  { to: '/tags', label: 'Tags' },
  { to: '/runs', label: 'Runs' },
] as const;

function StageBadge() {
  const { data } = useQuery<StageCountTask[]>({
    queryKey: ['tasks'],
    queryFn: () => api<StageCountTask[]>('/tasks'),
    retry: false,
    // Phase 08b will share this cache; just render gracefully on failure here.
  });
  const stagedCount = data?.filter((t) => t.staged_order !== null).length ?? 0;
  return (
    <Badge variant="secondary" data-testid="stage-badge">
      Stage ({stagedCount})
    </Badge>
  );
}

function TopNav() {
  return (
    <header className="sticky top-0 z-30 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-4 px-4">
        <Link
          to="/tasks"
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
              className="rounded-md px-3 py-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
              activeProps={{
                className: 'rounded-md px-3 py-1.5 bg-accent text-accent-foreground',
              }}
            >
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <StageBadge />
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
