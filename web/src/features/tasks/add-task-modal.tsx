/**
 * Spotlight-style create-task bar. Opened by:
 *   1. The visible "+ New task" button on the /tasks page header.
 *   2. The global `tt:new-task` CustomEvent (the `n` shortcut).
 *
 * Intentionally minimal — a single bar floating near the top of the screen
 * with no chrome. Enter creates the task and closes; Esc cancels (handled by
 * Radix Dialog). When `stageAfterCreate` is set (e.g. on /stage) the new task
 * is staged immediately so it lands in the user's current focus batch.
 */

import { Dialog as DialogPrimitive } from 'radix-ui';
import { useEffect, useRef, useState } from 'react';
import { useCreateTask, useStageTask } from '@/api/tasks';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';

export interface AddTaskModalProps {
  open: boolean;
  onOpenChange: (next: boolean) => void;
  /** When true, also stage the new task immediately after creating it. */
  stageAfterCreate?: boolean;
}

export function AddTaskModal({ open, onOpenChange, stageAfterCreate }: AddTaskModalProps) {
  const createTask = useCreateTask();
  const stageTask = useStageTask();
  const inputRef = useRef<HTMLInputElement>(null);
  const [title, setTitle] = useState('');

  useEffect(() => {
    if (!open) setTitle('');
  }, [open]);

  const submit = async () => {
    const trimmed = title.trim();
    if (!trimmed || createTask.isPending || stageTask.isPending) return;
    const created = await createTask.mutateAsync({ title: trimmed });
    if (stageAfterCreate) {
      await stageTask.mutateAsync(created.id);
    }
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="top-[20%] -translate-y-0 gap-0 p-0 sm:max-w-lg"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          inputRef.current?.focus();
        }}
      >
        <DialogPrimitive.Description className="sr-only">
          Type a title and press Enter to create a new task. Press Escape to cancel.
        </DialogPrimitive.Description>
        <DialogTitle className="sr-only">New task</DialogTitle>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <input
            ref={inputRef}
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="New task…"
            aria-label="New task title"
            className="w-full bg-transparent px-5 py-4 text-lg outline-none placeholder:text-muted-foreground"
          />
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Imperative listener for the global `tt:new-task` event (dispatched by the
 * `n` shortcut). Pages mount this so `n` opens their create modal.
 */
export function useNewTaskListener(onOpen: () => void) {
  useEffect(() => {
    const handler = () => onOpen();
    window.addEventListener('tt:new-task', handler);
    return () => window.removeEventListener('tt:new-task', handler);
  }, [onOpen]);
}
