/**
 * "Add task" dialog. Triggered by:
 *   1. The visible "+ New task" button on the /tasks page header.
 *   2. The global `tt:new-task` CustomEvent dispatched by the `n` shortcut.
 *
 * Submits via `useCreateTask()`. Validation is enforced by zod + RHF; an
 * empty title trips a visible error, mirroring the server's
 * `validation_failed` envelope so client and server stay in sync.
 */

import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { useCreateTask } from '@/api/tasks';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import type { TaskCreateInput } from '@/types/task';

const schema = z.object({
  title: z.string().trim().min(1, 'Title is required'),
  notes: z.string().optional(),
  due_date: z.string().optional(),
  tags: z.string().optional(),
});

type FormValues = z.infer<typeof schema>;

export interface AddTaskModalProps {
  open: boolean;
  onOpenChange: (next: boolean) => void;
}

export function AddTaskModal({ open, onOpenChange }: AddTaskModalProps) {
  const createTask = useCreateTask();
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { title: '', notes: '', due_date: '', tags: '' },
  });

  // Reset when the dialog closes so a stale draft doesn't leak into the
  // next "+ new task" interaction.
  useEffect(() => {
    if (!open) reset();
  }, [open, reset]);

  const onSubmit = handleSubmit(async (values) => {
    const input: TaskCreateInput = {
      title: values.title.trim(),
      notes: values.notes?.trim() || undefined,
      due_date: values.due_date?.trim() || null,
      tags: values.tags
        ? values.tags
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean)
        : undefined,
    };
    await createTask.mutateAsync(input);
    onOpenChange(false);
  });

  // Cmd/Ctrl-Enter submits from any field.
  const onKeyDown = (event: React.KeyboardEvent<HTMLFormElement>) => {
    if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
      event.preventDefault();
      void onSubmit();
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New task</DialogTitle>
        </DialogHeader>
        <form className="flex flex-col gap-3" onSubmit={onSubmit} onKeyDown={onKeyDown}>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="add-task-title">Title</Label>
            <Input id="add-task-title" autoFocus {...register('title')} />
            {errors.title && (
              <p className="text-xs text-destructive" role="alert">
                {errors.title.message}
              </p>
            )}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="add-task-notes">Notes</Label>
            <textarea
              id="add-task-notes"
              rows={3}
              className="min-h-16 rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
              {...register('notes')}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="add-task-due">Due date</Label>
              <Input id="add-task-due" type="date" {...register('due_date')} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="add-task-tags">Tags</Label>
              <Input id="add-task-tags" placeholder="comma,separated" {...register('tags')} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              Create task
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Imperative wrapper around `AddTaskModal` that opens itself in response to
 * the global `tt:new-task` event. Use this on the /tasks page so the `n`
 * keyboard shortcut works.
 */
export function useNewTaskListener(onOpen: () => void) {
  useEffect(() => {
    const handler = () => onOpen();
    window.addEventListener('tt:new-task', handler);
    return () => window.removeEventListener('tt:new-task', handler);
  }, [onOpen]);
}
