/**
 * Editor page tests. CodeMirror is mocked so we don't need a full browser
 * render context.
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
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import type { Script } from '@/types/script';
import { ScriptEditorPage } from './editor-page';

vi.mock('@uiw/react-codemirror', () => ({
  default: ({ value, onChange }: { value: string; onChange: (next: string) => void }) => (
    <textarea aria-label="Script code" value={value} onChange={(e) => onChange(e.target.value)} />
  ),
}));

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function script(partial: Partial<Script> & { id: number; name: string }): Script {
  return {
    code: '',
    enabled: true,
    schedule: { kind: 'daily' },
    last_run_at: null,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...partial,
  };
}

function renderEditor({ id, initial }: { id?: number; initial?: string } = {}) {
  const rootRoute = createRootRoute({ component: () => <Outlet /> });
  const newRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/new',
    component: () => <ScriptEditorPage />,
  });
  const detailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts/$id',
    component: function DetailHarness() {
      const params = detailRoute.useParams();
      const parsed = Number.parseInt(params.id, 10);
      return <ScriptEditorPage id={Number.isFinite(parsed) ? parsed : id} />;
    },
  });
  const listRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/scripts',
    component: () => null,
  });
  const runRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/runs/$id',
    component: () => null,
  });

  const router = createRouter({
    routeTree: rootRoute.addChildren([newRoute, detailRoute, listRoute, runRoute]),
    history: createMemoryHistory({
      initialEntries: [initial ?? (id !== undefined ? `/scripts/${id}` : '/scripts/new')],
    }),
  });
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    ...render(
      <QueryClientProvider client={qc}>
        <ThemeProvider>
          <RouterProvider router={router as never} />
        </ThemeProvider>
      </QueryClientProvider>,
    ),
    router,
  };
}

describe('ScriptEditorPage (new mode)', () => {
  afterEach(() => vi.restoreAllMocks());

  it('POSTs the right payload on submit', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return Promise.resolve(jsonResponse(script({ id: 9, name: 'My script' }), 201));
      }
      if (url === '/api/v1/scripts/9') {
        return Promise.resolve(jsonResponse(script({ id: 9, name: 'My script' })));
      }
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    renderEditor();

    const nameInput = await screen.findByLabelText('Name');
    await act(async () => {
      fireEvent.change(nameInput, { target: { value: 'My script' } });
    });

    const code = screen.getByLabelText('Script code');
    await act(async () => {
      fireEvent.change(code, { target: { value: 'ctx.log("hi")' } });
    });

    const submit = screen.getByRole('button', { name: 'Create script' });
    await act(async () => {
      fireEvent.click(submit);
    });

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        ([url, init]) => url === '/api/v1/scripts' && (init as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeTruthy();
      const body = JSON.parse((postCall?.[1] as RequestInit).body as string);
      expect(body).toEqual({
        name: 'My script',
        code: 'ctx.log("hi")',
        enabled: true,
        schedule: { kind: 'daily' },
      });
    });
  });

  it('shows a validation error when the name is empty', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    );
    renderEditor();
    await screen.findByLabelText('Name');
    const submit = screen.getByRole('button', { name: 'Create script' });
    await act(async () => {
      fireEvent.click(submit);
    });
    expect(await screen.findByText('Name is required')).toBeTruthy();
  });
});

describe('ScriptEditorPage (edit mode)', () => {
  afterEach(() => vi.restoreAllMocks());

  it('disables "Run now" when the form is dirty', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url === '/api/v1/scripts/7') {
        return Promise.resolve(jsonResponse(script({ id: 7, name: 'Existing', enabled: true })));
      }
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    renderEditor({ id: 7 });

    // Wait for the script to hydrate the form.
    const nameInput = await screen.findByDisplayValue('Existing');
    const runButton = (await screen.findByRole('button', { name: /Run now/ })) as HTMLButtonElement;
    // Before dirty: enabled.
    expect(runButton.disabled).toBe(false);

    await act(async () => {
      fireEvent.change(nameInput, { target: { value: 'Existing 2' } });
    });

    await waitFor(() => {
      expect(runButton.disabled).toBe(true);
    });
  });

  it('disables "Run now" when the script is disabled', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url === '/api/v1/scripts/8') {
        return Promise.resolve(jsonResponse(script({ id: 8, name: 'Off', enabled: false })));
      }
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    renderEditor({ id: 8 });
    await screen.findByDisplayValue('Off');
    const runButton = (await screen.findByRole('button', { name: /Run now/ })) as HTMLButtonElement;
    expect(runButton.disabled).toBe(true);
  });
});
