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

const SHORTCUTS: Shortcut[] = [
  { keys: 'n', description: 'Create new task (on Tasks page)' },
  { keys: '/', description: 'Focus search' },
  { keys: 'g t', description: 'Go to Tasks' },
  { keys: 'g s', description: 'Go to Stage' },
  { keys: 'g c', description: 'Go to Scripts' },
  { keys: 'g g', description: 'Go to Tags' },
  { keys: 'g r', description: 'Go to Runs' },
  { keys: '?', description: 'Toggle this cheatsheet' },
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
          <DialogDescription>Global shortcuts available throughout the app.</DialogDescription>
        </DialogHeader>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          {SHORTCUTS.map((s) => (
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
      </DialogContent>
    </Dialog>
  );
}
