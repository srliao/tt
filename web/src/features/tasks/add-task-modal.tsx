/**
 * Spotlight-style create-task bar. Opened by:
 *   1. The visible "+ New task" button on the /tasks page header.
 *   2. The global `tt:new-task` CustomEvent (the `n` shortcut).
 *
 * Intentionally minimal — a single bar floating near the top of the screen
 * with no chrome. Enter creates the task and closes; Esc cancels (handled by
 * Radix Dialog). When `stageAfterCreate` is set (e.g. on /stage) the new task
 * is staged immediately so it lands in the user's current focus batch.
 *
 * A "+ Add tags" disclosure lives below the title input. Clicking it reveals
 * a <TagCombobox>; the title-input fast path (Enter to submit) is preserved
 * because the form's onSubmit only fires when the title input owns submission
 * (the combobox's Enter is consumed by cmdk and does not propagate as a form
 * submit).
 */

import { Dialog as DialogPrimitive } from 'radix-ui';
import { useEffect, useRef, useState } from 'react';
import { useCreateTask, useStageTask } from '@/api/tasks';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { TagCombobox } from '@/components/ui/tag-combobox';

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
  const [tags, setTags] = useState<string[]>([]);
  const [showTags, setShowTags] = useState(false);

  useEffect(() => {
    if (!open) {
      setTitle('');
      setTags([]);
      setShowTags(false);
    }
  }, [open]);

  const submit = async () => {
    const trimmed = title.trim();
    if (!trimmed || createTask.isPending || stageTask.isPending) return;
    const created = await createTask.mutateAsync({
      title: trimmed,
      ...(tags.length > 0 ? { tags } : {}),
    });
    if (stageAfterCreate) {
      await stageTask.mutateAsync(created.id);
    }
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="top-[20%] translate-y-0 gap-0 p-0 sm:max-w-lg"
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
          <div className="flex flex-col gap-2 border-t border-border px-5 py-3">
            {showTags ? (
              <TagCombobox value={tags} onChange={setTags} allowCreate autoFocus />
            ) : (
              <button
                type="button"
                onClick={() => setShowTags(true)}
                className="self-start text-xs text-muted-foreground hover:text-foreground"
              >
                + Add tags
              </button>
            )}
          </div>
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
