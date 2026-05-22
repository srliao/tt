import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

interface Shortcut {
  keys: string;
  description: string;
}

interface ShortcutGroup {
  title: string;
  items: Shortcut[];
}

/**
 * Three-section keyboard reference. Per CLAUDE.md, every UI shortcut must be
 * discoverable here. When adding or removing a shortcut elsewhere, update the
 * matching group below in the same change.
 */
export const SHORTCUT_GROUPS: ShortcutGroup[] = [
  {
    title: 'Global',
    items: [
      { keys: '⌘ K', description: 'Open command palette' },
      { keys: '/', description: 'Open search (same palette)' },
      { keys: 'n', description: 'New task' },
      { keys: '?', description: 'This cheatsheet' },
    ],
  },
  {
    title: 'Navigate',
    items: [
      { keys: 'g t', description: 'Tasks' },
      { keys: 'g s', description: 'Stage' },
      { keys: 'g c', description: 'Scripts' },
      { keys: 'g g', description: 'Tags' },
      { keys: 'g r', description: 'Runs' },
    ],
  },
  {
    title: 'On a task',
    items: [
      { keys: 'j / k', description: 'Move focus down / up' },
      { keys: '↵', description: 'Edit task' },
      { keys: 'e', description: 'Edit task' },
      { keys: 'd', description: 'Toggle done' },
      { keys: 's', description: 'Stage / unstage' },
      { keys: 't', description: 'Edit tags inline' },
    ],
  },
  {
    title: 'Selection',
    items: [
      { keys: 'x', description: 'Select / deselect focused task' },
      { keys: '␣', description: 'Same — Space also works' },
      { keys: '⇧ j / ⇧ k', description: 'Extend range' },
      { keys: '⌘ A', description: 'Select all visible' },
      { keys: '⇧ ⌘ A', description: 'Select all matching the filter' },
      { keys: 'Esc', description: 'Clear selection' },
    ],
  },
];

export function ShortcutCheatsheet() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const handler = () => setOpen((v) => !v);
    window.addEventListener('tt:toggle-cheatsheet', handler);
    return () => window.removeEventListener('tt:toggle-cheatsheet', handler);
  }, []);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>
            Keys available throughout the app. Single-letter shortcuts only fire when you aren't
            typing in a text field.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          {SHORTCUT_GROUPS.map((group) => (
            <section key={group.title} className="flex flex-col gap-2">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {group.title}
              </h3>
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                {group.items.map((s) => (
                  <div key={s.keys} className="contents">
                    <dt>
                      <kbd className="rounded border bg-muted px-2 py-0.5 font-mono text-xs">
                        {s.keys}
                      </kbd>
                    </dt>
                    <dd className="text-muted-foreground">{s.description}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
