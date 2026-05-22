/**
 * Tag cheatsheet — lists every existing tag from `useTags()` and lets the
 * user copy a quoted name onto the clipboard (so they can paste it
 * straight into a `tags: [...]` array in their script body).
 *
 * The "quoted name" detail matters: scripts use `tags: ["weekly"]`, so
 * copying the bare name would force the user to manually wrap quotes
 * around every paste. Wrapping in double quotes here makes the panel a
 * true convenience.
 */

import { CopyIcon } from 'lucide-react';
import { useState } from 'react';
import { useTags } from '@/api/tags';
import { Button } from '@/components/ui/button';

export function CheatsheetTags() {
  const { data: tags = [], isLoading } = useTags();
  const [copied, setCopied] = useState<string | null>(null);

  const onCopy = async (name: string) => {
    const quoted = `"${name}"`;
    try {
      await navigator.clipboard.writeText(quoted);
      setCopied(name);
      setTimeout(() => setCopied((cur) => (cur === name ? null : cur)), 1200);
    } catch {
      // Locked-down clipboard envs — silently ignore.
    }
  };

  if (isLoading) {
    return <p className="text-xs text-muted-foreground">Loading tags…</p>;
  }

  if (tags.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No tags yet. Create some on the Tags page and they'll show up here.
      </p>
    );
  }

  const sorted = [...tags].sort((a, b) => a.name.localeCompare(b.name));

  return (
    <div className="flex flex-col gap-2">
      <p className="text-xs text-muted-foreground">
        Click the copy icon to drop <code>&quot;name&quot;</code> into the clipboard, ready to paste
        into a <code>tags: [...]</code> array.
      </p>
      <ul className="flex flex-col gap-1" aria-label="Tags">
        {sorted.map((tag) => (
          <li
            key={tag.id}
            className="flex items-center justify-between gap-2 rounded-md border bg-card px-2 py-1.5"
          >
            <code className="truncate font-mono text-xs">{tag.name}</code>
            <div className="flex items-center gap-2">
              {copied === tag.name && (
                <span className="text-[10px] text-muted-foreground">Copied!</span>
              )}
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={`Copy "${tag.name}"`}
                onClick={() => void onCopy(tag.name)}
              >
                <CopyIcon />
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
