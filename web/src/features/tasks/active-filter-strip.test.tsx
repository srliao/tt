/**
 * Tests for <ActiveFilterStrip>.
 *
 * The component reads/writes the /tasks URL search-params via
 * `useTaskListSearch`, so we mount it inside a TanStack memory router that
 * mirrors the real route's `validateSearch`. Each test seeds the URL with a
 * specific filter combination and asserts both rendering and the URL effect
 * of remove-chip / clear-all interactions.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import { ActiveFilterStrip } from './active-filter-strip';
import { taskSearchSchema } from './use-task-list-search';

function renderStrip(initial = '/tasks') {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    validateSearch: (s) => taskSearchSchema.parse(s),
    component: () => <ActiveFilterStrip />,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([tasksRoute]),
    history: createMemoryHistory({ initialEntries: [initial] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const result = render(
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        <RouterProvider router={router as never} />
      </QueryClientProvider>
    </ThemeProvider>,
  );
  return { router, ...result };
}

describe('ActiveFilterStrip', () => {
  it('renders the "Open only · include done?" pill on a fresh /tasks visit', async () => {
    // Default view (states unset) is filtered to not_done — the strip mounts
    // with the include-done affordance so first-time users can discover the
    // hidden states. No other chips should appear.
    const { container } = renderStrip('/tasks');
    await screen.findByRole('button', { name: 'Open only · include done?' });
    expect(container.querySelector('[data-slot="active-filter-strip"]')).not.toBeNull();
    expect(container.querySelector('[data-slot="filter-chip"]')).toBeNull();
  });

  it('hides the include-done pill once all three states are explicitly selected', async () => {
    const { router } = renderStrip('/tasks');
    await act(async () => {
      await router.navigate({
        to: '/tasks',
        search: { states: ['not_done', 'done', 'cancelled'] },
      });
    });
    // The strip itself still mounts (states is a non-empty filter axis), but
    // the "Open only" affordance is gone because nothing is hidden.
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Open only · include done?' })).toBeNull();
    });
  });

  it('renders a q chip and removes it on click', async () => {
    const { router } = renderStrip('/tasks?q=milk');
    await screen.findByText('"milk"');

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Remove filter "milk"' }));
    });

    await waitFor(() => {
      const s = router.state.location.search as { q?: string };
      expect(s.q).toBeUndefined();
    });
  });

  it('renders a due chip and removes it on click', async () => {
    const { router } = renderStrip('/tasks?due=today');
    await screen.findByText('due: today');

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Remove filter due: today' }));
    });

    await waitFor(() => {
      const s = router.state.location.search as { due?: string };
      expect(s.due).toBeUndefined();
    });
  });

  it('renders include + exclude tag chips and removes individual ones', async () => {
    const { router } = renderStrip('/tasks');
    await act(async () => {
      await router.navigate({
        to: '/tasks',
        search: { tags: ['work', 'urgent'], tagsExclude: ['noise'] },
      });
    });

    // Both included tags + the excluded one show up.
    await screen.findByText('work');
    expect(screen.getByText('urgent')).toBeTruthy();
    expect(screen.getByText('noise')).toBeTruthy();

    // Remove the included "work" tag.
    const removeButtons = screen.getAllByRole('button', { name: 'Remove work' });
    await act(async () => {
      fireEvent.click(removeButtons[0]);
    });

    await waitFor(() => {
      const s = router.state.location.search as {
        tags?: string[];
        tagsExclude?: string[];
      };
      expect(s.tags).toEqual(['urgent']);
      expect(s.tagsExclude).toEqual(['noise']);
    });
  });

  it('shows the state-restricted chip and widens states on click', async () => {
    const { router } = renderStrip('/tasks');
    await act(async () => {
      await router.navigate({ to: '/tasks', search: { states: ['not_done'] } });
    });
    const chip = await screen.findByRole('button', { name: 'Open only · include done?' });

    await act(async () => {
      fireEvent.click(chip);
    });

    await waitFor(() => {
      const s = router.state.location.search as { states?: string[] };
      expect(s.states).toEqual(['not_done', 'done', 'cancelled']);
    });
  });

  it('Clear all wipes every filter axis including the quick preset', async () => {
    const { router } = renderStrip('/tasks');
    await act(async () => {
      await router.navigate({
        to: '/tasks',
        search: {
          q: 'foo',
          due: 'today',
          tags: ['work'],
          tagsExclude: ['noise'],
          quick: 'overdue',
          states: ['not_done'],
        },
      });
    });

    await screen.findByText('"foo"');
    const clear = screen.getByRole('button', { name: 'Clear all' });
    await act(async () => {
      fireEvent.click(clear);
    });

    await waitFor(() => {
      const s = router.state.location.search as Record<string, unknown>;
      expect(s.q).toBeUndefined();
      expect(s.due).toBeUndefined();
      expect(s.tags).toBeUndefined();
      expect(s.tagsExclude).toBeUndefined();
      expect(s.quick).toBeUndefined();
      expect(s.states).toBeUndefined();
    });
  });
});
