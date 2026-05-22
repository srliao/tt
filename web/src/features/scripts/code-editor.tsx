/**
 * Thin wrapper around `@uiw/react-codemirror` that:
 *
 * - Loads the JavaScript language extension.
 * - Picks the light/dark theme from our `useTheme()` provider so the editor
 *   matches the rest of the app on theme switches.
 * - Constrains height to `min-h-[50vh]` so the editor has presence even on a
 *   freshly-created script.
 *
 * The editor itself is uncontrolled-ish (CodeMirror keeps its own document
 * state) but mirrors changes back through `onChange`, which the parent form
 * pipes into react-hook-form via a `<Controller>`.
 */

import { javascript } from '@codemirror/lang-javascript';
import CodeMirror from '@uiw/react-codemirror';
import { useTheme } from '@/components/theme-provider';

export interface CodeEditorProps {
  value: string;
  onChange: (next: string) => void;
  /** Optional accessible name; falls back to "Script code". */
  ariaLabel?: string;
}

export function CodeEditor({ value, onChange, ariaLabel }: CodeEditorProps) {
  const { resolvedTheme } = useTheme();
  return (
    <div
      data-script-code
      data-cm-label={ariaLabel ?? 'Script code'}
      className="min-h-[50vh] overflow-hidden rounded-md border"
    >
      <CodeMirror
        value={value}
        onChange={onChange}
        extensions={[javascript()]}
        theme={resolvedTheme === 'dark' ? 'dark' : 'light'}
        height="auto"
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: true,
        }}
        className="min-h-[50vh] text-sm"
      />
    </div>
  );
}
