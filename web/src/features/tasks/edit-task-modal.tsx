/**
 * Edit-task dialog. Opened by clicking a task title on the /tasks or /stage
 * pages. Exposes the full editable surface — title, notes, due date, tags —
 * and submits via `useUpdateTask()`. Also surfaces the two non-form actions
 * "Mark cancelled" and "Delete" (with a confirm prompt) since the row UI no
 * longer has a kebab menu.
 *
 * Task creation lives elsewhere (see `add-task-modal.tsx`); this component is
 * edit-only and requires a `task` prop.
 */

import { zodResolver } from '@hookform/resolvers/zod';
import { Trash2Icon, XCircleIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { useDeleteTask, useSetTaskState, useUpdateTask } from '@/api/tasks';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
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
import type { Task, TaskUpdateInput } from '@/types/task';

const schema = z.object({
  title: z.string().trim().min(1, 'Title is required'),
  notes: z.string().optional(),
  due_date: z.string().optional(),
  tags: z.string().optional(),
});

type FormValues = z.infer<typeof schema>;

export interface EditTaskModalProps {
  /** Falsy `task` keeps the dialog closed; truthy opens it. */
  task: Task | null;
  onOpenChange: (next: boolean) => void;
}

function taskToFormValues(task: Task | null): FormValues {
  if (!task) return { title: '', notes: '', due_date: '', tags: '' };
  return {
    title: task.title,
    notes: task.notes ?? '',
    due_date: task.due_date ?? '',
    tags: (task.tags ?? []).join(', '),
  };
}

export function EditTaskModal({ task, onOpenChange }: EditTaskModalProps) {
  const updateTask = useUpdateTask();
  const setState = useSetTaskState();
  const deleteTask = useDeleteTask();
  const open = !!task;
  const [confirmDelete, setConfirmDelete] = useState(false);
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: taskToFormValues(task),
  });

  useEffect(() => {
    if (open) {
      reset(taskToFormValues(task));
    } else {
      reset({ title: '', notes: '', due_date: '', tags: '' });
      setConfirmDelete(false);
    }
  }, [open, task, reset]);

  const onSubmit = handleSubmit(async (values) => {
    if (!task) return;
    const tags = values.tags
      ? values.tags
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean)
      : [];
    const input: TaskUpdateInput = {
      title: values.title.trim(),
      notes: values.notes?.trim() ?? '',
      due_date: values.due_date?.trim() || null,
      tags,
    };
    await updateTask.mutateAsync({ id: task.id, input });
    onOpenChange(false);
  });

  // Cmd/Ctrl-Enter submits from the notes textarea (Enter alone inserts a newline there).
  const onKeyDown = (event: React.KeyboardEvent<HTMLFormElement>) => {
    if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
      event.preventDefault();
      void onSubmit();
    }
  };

  const cancelled = task?.state === 'cancelled';
  const onToggleCancelled = () => {
    if (!task) return;
    setState.mutate({ id: task.id, state: cancelled ? 'not_done' : 'cancelled' });
    onOpenChange(false);
  };

  const onDelete = () => {
    if (!task) return;
    deleteTask.mutate(task.id);
    setConfirmDelete(false);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit task</DialogTitle>
        </DialogHeader>
        <form className="flex flex-col gap-3" onSubmit={onSubmit} onKeyDown={onKeyDown}>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-task-title">Title</Label>
            <Input id="edit-task-title" autoFocus {...register('title')} />
            {errors.title && (
              <p className="text-xs text-destructive" role="alert">
                {errors.title.message}
              </p>
            )}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="edit-task-notes">Notes</Label>
            <textarea
              id="edit-task-notes"
              rows={3}
              className="min-h-16 rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
              {...register('notes')}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-task-due">Due date</Label>
              <Input id="edit-task-due" type="date" {...register('due_date')} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-task-tags">Tags</Label>
              <Input id="edit-task-tags" placeholder="comma,separated" {...register('tags')} />
            </div>
          </div>
          <DialogFooter className="flex-wrap sm:justify-between">
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={onToggleCancelled}>
                <XCircleIcon /> {cancelled ? 'Un-cancel' : 'Mark cancelled'}
              </Button>
              <Button type="button" variant="destructive" onClick={() => setConfirmDelete(true)}>
                <Trash2Icon /> Delete
              </Button>
            </div>
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={isSubmitting}>
                Save changes
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this task?</AlertDialogTitle>
            <AlertDialogDescription>
              This cannot be undone. The task will be permanently removed.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={onDelete}>
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  );
}
