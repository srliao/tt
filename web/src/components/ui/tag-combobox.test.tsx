/**
 * Unit tests for <TagCombobox>.
 *
 * The component depends on `useTagsWithCounts` (tag list) and `useCreateTag`
 * (server POST). Tests pass `available` explicitly so they don't need a
 * fetch mock for the listing, but exercise create with a stubbed global
 * fetch so we can assert the network call shape.
 */

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ThemeProvider } from '@/components/theme-provider';
import { TagCombobox } from '@/components/ui/tag-combobox';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function renderCombobox(
  props: Partial<React.ComponentProps<typeof TagCombobox>> & {
    initialValue?: string[];
  } = {},
) {
  const { initialValue = [], ...rest } = props;
  const onChange = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const utils = render(
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        <TagCombobox
          value={initialValue}
          onChange={onChange}
          available={[
            { name: 'home', count: 3 },
            { name: 'work', count: 7 },
            { name: 'errand', count: 1 },
          ]}
          {...rest}
        />
      </QueryClientProvider>
    </ThemeProvider>,
  );
  return { onChange, ...utils };
}

function getInput() {
  return document.querySelector('[data-slot="tag-combobox-input"]') as HTMLInputElement;
}

function getTrigger() {
  return document.querySelector('[data-slot="tag-combobox-trigger"]') as HTMLElement;
}

describe('TagCombobox', () => {
  afterEach(() => vi.restoreAllMocks());

  it('renders chips for initial value', () => {
    renderCombobox({ initialValue: ['home', 'work'] });
    expect(screen.getByText('home')).toBeInTheDocument();
    expect(screen.getByText('work')).toBeInTheDocument();
  });

  it('opens the popover and filters the list when typing', async () => {
    renderCombobox();
    const input = getInput();
    act(() => {
      input.focus();
    });
    await act(async () => {
      fireEvent.change(input, { target: { value: 'wo' } });
    });
    // 'work' should match; 'home' should not appear.
    await waitFor(() => {
      const items = screen.getAllByRole('option');
      const names = items.map((el) => el.getAttribute('data-tag-name'));
      expect(names).toContain('work');
      expect(names).not.toContain('home');
    });
  });

  it('toggling an existing tag adds it to onChange', async () => {
    const { onChange } = renderCombobox();
    const input = getInput();
    act(() => {
      input.focus();
    });
    // Wait for the dropdown items to render
    await screen.findByRole('option', { name: /home/i });
    const homeItem = screen.getByRole('option', { name: /home/i });
    await act(async () => {
      fireEvent.click(homeItem);
    });
    expect(onChange).toHaveBeenCalledWith(['home']);
  });

  it('toggling an already-selected tag removes it', async () => {
    const { onChange } = renderCombobox({ initialValue: ['work'] });
    const input = getInput();
    act(() => {
      input.focus();
    });
    const workItem = await screen.findByRole('option', { name: /work/i });
    await act(async () => {
      fireEvent.click(workItem);
    });
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it('pasting "a, b" splits and adds each tag', async () => {
    const { onChange } = renderCombobox();
    const input = getInput();
    act(() => {
      input.focus();
    });
    await act(async () => {
      fireEvent.paste(input, {
        clipboardData: {
          getData: () => 'alpha, beta',
        },
      });
    });
    expect(onChange).toHaveBeenCalledWith(['alpha', 'beta']);
  });

  it('backspace on empty input removes the last chip', async () => {
    const { onChange } = renderCombobox({ initialValue: ['home', 'work'] });
    const input = getInput();
    act(() => {
      input.focus();
    });
    await act(async () => {
      fireEvent.keyDown(input, { key: 'Backspace' });
    });
    expect(onChange).toHaveBeenCalledWith(['home']);
  });

  it('with allowCreate=false and no exact match, the Create row is absent', async () => {
    renderCombobox({ allowCreate: false });
    const input = getInput();
    act(() => {
      input.focus();
    });
    await act(async () => {
      fireEvent.change(input, { target: { value: 'brandnew' } });
    });
    await waitFor(() => {
      // The component renders the placeholder text "No tags found." when
      // filter has zero matches and create is not allowed.
      expect(screen.getByText(/no tags found/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/create/i)).not.toBeInTheDocument();
  });

  it('with allowCreate=true and a unique query, the Create row appears and selecting it calls POST /tags', async () => {
    const fetchMock = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return Promise.resolve(
          jsonResponse({ id: 99, name: 'brandnew', created_at: '2026-05-22T00:00:00Z' }),
        );
      }
      // Invalidation refetches `/tags?counts=1` etc. — return empty list.
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal('fetch', fetchMock);

    const { onChange } = renderCombobox({ allowCreate: true });
    const input = getInput();
    act(() => {
      input.focus();
    });
    await act(async () => {
      fireEvent.change(input, { target: { value: 'brandnew' } });
    });

    const createRow = await screen.findByText(/Create/i);
    expect(createRow).toBeInTheDocument();

    // Click the create row's parent <option>
    const option = createRow.closest('[role="option"]') as HTMLElement;
    expect(option).not.toBeNull();
    await act(async () => {
      fireEvent.click(option);
    });

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
      );
      expect(postCall).toBeTruthy();
      const [url, init] = postCall as [string, RequestInit];
      expect(String(url)).toContain('/tags');
      expect(String(init.body)).toContain('brandnew');
    });

    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith(['brandnew']);
    });
  });

  it('Escape closes the popover without clearing value', async () => {
    const { onChange } = renderCombobox({ initialValue: ['home'] });
    const input = getInput();
    act(() => {
      input.focus();
    });
    await act(async () => {
      fireEvent.change(input, { target: { value: 'wo' } });
    });
    await act(async () => {
      fireEvent.keyDown(input, { key: 'Escape' });
    });
    // onChange must not have been called as a side-effect of Escape
    expect(onChange).not.toHaveBeenCalled();
    // The original chip remains
    expect(screen.getByText('home')).toBeInTheDocument();
  });

  it('clicking a chip × removes that single chip', () => {
    const { onChange } = renderCombobox({ initialValue: ['home', 'work', 'errand'] });
    const trigger = getTrigger();
    const removeWork = within(trigger).getByRole('button', { name: /Remove work/i });
    act(() => {
      fireEvent.click(removeWork);
    });
    expect(onChange).toHaveBeenCalledWith(['home', 'errand']);
  });

  it('does not show a Create row when the typed query exactly matches an existing tag', async () => {
    renderCombobox();
    const input = getInput();
    act(() => {
      input.focus();
    });
    await act(async () => {
      fireEvent.change(input, { target: { value: 'home' } });
    });
    await screen.findByRole('option', { name: /home/i });
    expect(screen.queryByText(/^Create /i)).not.toBeInTheDocument();
  });
});
