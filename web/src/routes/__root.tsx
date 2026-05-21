import { createRootRoute, Outlet, useRouter } from '@tanstack/react-router';
import { lazy, Suspense } from 'react';
import { AppLayout } from '@/components/layout';
import { useGlobalShortcuts } from '@/lib/shortcuts';

const TanStackRouterDevtools = import.meta.env.DEV
  ? lazy(() =>
      import('@tanstack/router-devtools').then((mod) => ({
        default: mod.TanStackRouterDevtools,
      })),
    )
  : () => null;

const ReactQueryDevtools = import.meta.env.DEV
  ? lazy(() =>
      import('@tanstack/react-query-devtools').then((mod) => ({
        default: mod.ReactQueryDevtools,
      })),
    )
  : () => null;

function RootComponent() {
  const router = useRouter();
  // The router has typed access via the Register declaration in src/router.tsx.
  useGlobalShortcuts(router as unknown as typeof import('@/router').router);
  return (
    <AppLayout>
      <Outlet />
      {import.meta.env.DEV && (
        <Suspense fallback={null}>
          <TanStackRouterDevtools position="bottom-right" />
          <ReactQueryDevtools initialIsOpen={false} />
        </Suspense>
      )}
    </AppLayout>
  );
}

export const Route = createRootRoute({
  component: RootComponent,
});
