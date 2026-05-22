import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/components/theme-provider';
import { CodeEditor } from './code-editor';

// CodeMirror tries to measure its container with ResizeObserver and
// drawing APIs that don't exist in jsdom. We replace the heavy import
// with a stub that records the props it received so we can assert on
// the wiring without actually rendering the editor.
vi.mock('@uiw/react-codemirror', () => ({
  default: ({
    value,
    onChange,
    theme,
  }: {
    value: string;
    onChange: (next: string) => void;
    theme: string | 'light' | 'dark';
  }) => (
    <div data-testid="cm-stub" data-theme={String(theme)}>
      <textarea
        aria-label="cm-stub-input"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  ),
}));

describe('CodeEditor', () => {
  it('forwards value/theme into the CodeMirror primitive', () => {
    render(
      <ThemeProvider>
        <CodeEditor value="ctx.queueTask({title:'x'})" onChange={() => {}} />
      </ThemeProvider>,
    );
    const stub = screen.getByTestId('cm-stub');
    expect(stub.getAttribute('data-theme')).toMatch(/light|dark/);
    expect(screen.getByLabelText('cm-stub-input')).toHaveValue("ctx.queueTask({title:'x'})");
  });
});
