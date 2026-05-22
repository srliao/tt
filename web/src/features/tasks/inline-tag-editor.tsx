/**
 * <InlineTagEditor> — keyboard partner to clicking a tag chip.
 *
 * Opened by pressing `t` on a focused task row. Renders a `TagCombobox`
 * popover anchored to that row's `[data-tag-cell]` element. On close (Esc
 * or click outside) any pending tag edits are committed via
 * `useUpdateTask`. Keeps the heavy edit modal out of the way for the common
 * "I just want to relabel this task" flow.
 *
 * The editor is intentionally controlled by the parent — the parent owns
 * the `task` (null when closed) and clears it on `onClose`.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { useUpdateTask } from '@/api/tasks';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { TagCombobox } from '@/components/ui/tag-combobox';
import type { Task } from '@/types/task';

export interface InlineTagEditorProps {
  /** Falsy = closed. Truthy = open over `[data-task-id=task.id] [data-tag-cell]`. */
  task: Task | null;
  onClose: () => void;
}

export function InlineTagEditor({ task, onClose }: InlineTagEditorProps) {
  const updateTask = useUpdateTask();
  const [tags, setTags] = useState<string[]>(task?.tags ?? []);
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  // Keep the latest tag set in a ref so the unmount/close commit can read
  // the post-edit value without needing it as a dep of the effect.
  const tagsRef = useRef(tags);
  tagsRef.current = tags;
  // Same trick for the task — we need to know which task to commit against
  // even after `task` flips to null on close.
  const lastTaskRef = useRef<Task | null>(task);

  // When a new task becomes the editor target, seed the local state from
  // its current tags and resolve the DOM anchor.
  useEffect(() => {
    if (!task) {
      setAnchorEl(null);
      return;
    }
    lastTaskRef.current = task;
    setTags(task.tags ?? []);
    const el = document.querySelector<HTMLElement>(`[data-task-id="${task.id}"] [data-tag-cell]`);
    setAnchorEl(el);
  }, [task]);

  const [error, setError] = useState<string | null>(null);

  const commitAndClose = useCallback(async () => {
    const original = lastTaskRef.current;
    if (!original) {
      onClose();
      return;
    }
    const next = tagsRef.current;
    // Skip the network round-trip if nothing changed (also avoids
    // clobbering a concurrent edit with stale data).
    const same =
      next.length === (original.tags?.length ?? 0) && next.every((t, i) => t === original.tags[i]);
    if (same) {
      onClose();
      return;
    }
    try {
      await updateTask.mutateAsync({
        id: original.id,
        input: {
          title: original.title,
          notes: original.notes ?? '',
          due_date: original.due_date,
          tags: next,
        },
      });
      setError(null);
      onClose();
    } catch (err) {
      // No global toast surface exists yet — keep the editor open and
      // render an inline error so the user can retry or hit Escape again
      // to dismiss without retrying. Better than silently losing the edit.
      console.error('Failed to save tag edits:', err);
      setError(err instanceof Error ? err.message : 'Failed to save tags');
    }
  }, [onClose, updateTask]);

  // While open, intercept Escape at the document level (capture phase) so
  // we run before the nested TagCombobox popover swallows it as its own
  // dismissal. Without this, the inner popover closes silently and our
  // commit-and-close path never fires.
  useEffect(() => {
    if (!task) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        // If an error is showing, a second Escape force-dismisses the
        // editor (discarding the pending edit) instead of retrying.
        if (error) {
          setError(null);
          onClose();
          return;
        }
        void commitAndClose();
      }
    };
    document.addEventListener('keydown', onKeyDown, { capture: true });
    return () => document.removeEventListener('keydown', onKeyDown, { capture: true });
  }, [task, commitAndClose, error, onClose]);

  if (!task || !anchorEl) return null;

  return (
    <Popover
      open
      onOpenChange={(next) => {
        if (!next) void commitAndClose();
      }}
    >
      <PopoverAnchor virtualRef={{ current: anchorEl }} />
      <PopoverContent
        align="start"
        sideOffset={4}
        className="w-80 p-2"
        data-slot="inline-tag-editor"
      >
        <TagCombobox value={tags} onChange={setTags} allowCreate />
        {error && (
          <p
            className="mt-2 text-xs text-destructive"
            role="alert"
            data-slot="inline-tag-editor-error"
          >
            {error}. Press Escape again to discard.
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}
