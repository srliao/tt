import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it } from 'vitest';
import type { SpawnedTaskSummary } from '@/api/runs';
import { SpawnedTasksChips } from './spawned-tasks-chips';

function renderWithRouter(node: ReactNode) {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const here = createRoute({
    getParentRoute: () => rootRoute,
    path: '/',
    component: () => <>{node}</>,
  });
  const tasksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/tasks',
    component: () => null,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([here, tasksRoute]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  });
  return render(<RouterProvider router={router as never} />);
}

const tasks: SpawnedTaskSummary[] = [
  { id: 1, title: 'Write summary', state: 'not_done' },
  { id: 2, title: 'Send email', state: 'done' },
  { id: 3, title: 'Cancelled item', state: 'cancelled' },
];

describe('SpawnedTasksChips', () => {
  it('renders one chip per existing task with its label', async () => {
    renderWithRouter(<SpawnedTasksChips spawnedIds={[1, 2, 3]} tasks={tasks} />);
    expect(await screen.findByText('Write summary')).toBeTruthy();
    expect(screen.getByText('Send email')).toBeTruthy();
    expect(screen.getByText('Cancelled item')).toBeTruthy();
    expect(screen.getByText('done')).toBeTruthy();
  });

  it('renders a deleted chip for ids not present in the tasks list', async () => {
    renderWithRouter(<SpawnedTasksChips spawnedIds={[1, 99]} tasks={[tasks[0]]} />);
    expect(await screen.findByText('Task #99')).toBeTruthy();
    expect(screen.getByText('deleted')).toBeTruthy();
  });

  it('handles an empty spawned list cleanly', () => {
    const { container } = renderWithRouter(<SpawnedTasksChips spawnedIds={[]} tasks={[]} />);
    expect(container.querySelectorAll('li')).toHaveLength(0);
  });
});
