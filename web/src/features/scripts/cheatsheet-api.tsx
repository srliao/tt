/**
 * Renders the ctx API cheatsheet from a bundled markdown file. The file is
 * authored as a plain markdown reference (one `## ctx.foo()` heading per
 * function followed by a one-line description). We don't ship a markdown
 * renderer; instead we parse it ourselves so each function entry can carry
 * a "copy" button that writes the function name to the clipboard.
 *
 * Why a custom parser: we only need H2 headings + their body, and avoiding
 * `marked`/`react-markdown` keeps the SPA bundle small.
 */

import { CopyIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import md from '@/assets/ctx-cheatsheet.md?raw';
import { Button } from '@/components/ui/button';

interface CheatsheetEntry {
  /** The function name (e.g. `ctx.today()`). */
  name: string;
  /** The plain-text body lines under the heading (newlines joined with a space). */
  description: string;
}

/**
 * Splits the cheatsheet markdown into one entry per `## ` heading. Exported
 * for tests so we don't need to mock the `?raw` import.
 */
export function parseCheatsheet(source: string): CheatsheetEntry[] {
  const lines = source.split('\n');
  const entries: CheatsheetEntry[] = [];
  let current: CheatsheetEntry | null = null;
  for (const line of lines) {
    if (line.startsWith('## ')) {
      if (current) entries.push(finalize(current));
      current = { name: line.slice(3).trim(), description: '' };
    } else if (current && !line.startsWith('# ')) {
      if (line.trim().length > 0) {
        current.description += (current.description ? ' ' : '') + line.trim();
      }
    }
  }
  if (current) entries.push(finalize(current));
  return entries;
}

function finalize(entry: CheatsheetEntry): CheatsheetEntry {
  return { ...entry, description: entry.description.trim() };
}

export function CheatsheetApi() {
  const entries = useMemo(() => parseCheatsheet(md), []);
  const [copied, setCopied] = useState<string | null>(null);

  const onCopy = async (name: string) => {
    // Strip the trailing parenthesised signature so the copied snippet is
    // just `ctx.today()` -> still valid JS to paste into a script body.
    try {
      await navigator.clipboard.writeText(name);
      setCopied(name);
      setTimeout(() => setCopied((cur) => (cur === name ? null : cur)), 1200);
    } catch {
      // Older browsers / locked-down envs — silently ignore.
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <p className="text-xs text-muted-foreground">
        Click the copy icon to drop a function name into the clipboard.
      </p>
      <ul className="flex flex-col gap-2" aria-label="ctx API">
        {entries.map((entry) => (
          <li
            key={entry.name}
            className="flex flex-col gap-1 rounded-md border bg-card px-2 py-1.5"
          >
            <div className="flex items-center justify-between gap-2">
              <code className="font-mono text-xs">{entry.name}</code>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={`Copy ${entry.name}`}
                onClick={() => void onCopy(entry.name)}
              >
                <CopyIcon />
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">{entry.description}</p>
            {copied === entry.name && (
              <span className="text-[10px] text-muted-foreground">Copied!</span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
