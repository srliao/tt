/**
 * Single tag row rendered inside the /tags page list.
 *
 * - Click the pencil icon to switch the name into an `<Input>`; Enter saves
 *   (calls `useRenameTag`), Esc cancels.
 * - Click the trash icon to open an `<AlertDialog>` warning that deletion
 *   cascades through `task_tags`. Confirming triggers `useDeleteTag`.
 *
 * The cascade-confirm copy comes from spec §6: "This will remove the tag from
 * any tasks that use it. Continue?".
 */

import { PencilIcon, Trash2Icon } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useDeleteTag, useRenameTag } from '@/api/tags';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ApiError } from '@/lib/api';
import type { Tag } from '@/types/tag';

export interface TagRowProps {
  tag: Tag;
}

export function TagRow({ tag }: TagRowProps) {
  const rename = useRenameTag();
  const del = useDeleteTag();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(tag.name);
  const [error, setError] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) {
      setDraft(tag.name);
      setError(null);
      // Focus + select-all so a quick keystroke replaces the existing name.
      const id = requestAnimationFrame(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      });
      return () => cancelAnimationFrame(id);
    }
  }, [editing, tag.name]);

  const startEdit = () => setEditing(true);
  const cancelEdit = () => {
    setEditing(false);
    setDraft(tag.name);
    setError(null);
  };

  const commitEdit = async () => {
    const next = draft.trim();
    if (next.length === 0) {
      setError('Tag name is required');
      return;
    }
    if (next === tag.name) {
      cancelEdit();
      return;
    }
    setError(null);
    try {
      await rename.mutateAsync({ id: tag.id, name: next });
      setEditing(false);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'conflict') {
        setError('Tag already exists');
        return;
      }
      throw err;
    }
  };

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      void commitEdit();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      cancelEdit();
    }
  };

  return (
    <li
      data-tag-id={tag.id}
      className="flex items-center gap-2 rounded-md border bg-card px-3 py-2 text-sm"
    >
      {editing ? (
        <div className="flex flex-1 flex-col gap-1">
          <Input
            ref={inputRef}
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              if (error) setError(null);
            }}
            onKeyDown={onKeyDown}
            onBlur={cancelEdit}
            aria-label={`Rename ${tag.name}`}
            aria-invalid={!!error}
          />
          {error && (
            <p className="text-xs text-destructive" role="alert">
              {error}
            </p>
          )}
        </div>
      ) : (
        <span className="flex-1 truncate font-medium">{tag.name}</span>
      )}

      {!editing && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={`Edit ${tag.name}`}
          onClick={startEdit}
        >
          <PencilIcon />
        </Button>
      )}

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogTrigger asChild>
          <Button type="button" variant="ghost" size="icon-sm" aria-label={`Delete ${tag.name}`}>
            <Trash2Icon />
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete tag "{tag.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove the tag from any tasks that use it. Continue?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                del.mutate(tag.id);
                setConfirmOpen(false);
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </li>
  );
}
