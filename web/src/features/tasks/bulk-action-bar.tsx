/**
 * Sticky bottom bar that surfaces bulk actions whenever ≥1 row in the task
 * table is selected. Each button fires the relevant mutation hook once per
 * selected ID — the v1 API does not have bulk endpoints, so we loop.
 */

import { useState } from 'react';
import { useDeleteTask, useSetTaskState, useStageTask } from '@/api/tasks';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

export interface BulkActionBarProps {
  selectedIds: Set<number>;
  onClear: () => void;
}

export function BulkActionBar({ selectedIds, onClear }: BulkActionBarProps) {
  const setState = useSetTaskState();
  const stage = useStageTask();
  const del = useDeleteTask();
  const [confirmDelete, setConfirmDelete] = useState(false);

  if (selectedIds.size === 0) return null;
  const ids = Array.from(selectedIds);

  const markDone = () => {
    for (const id of ids) {
      setState.mutate({ id, state: 'done' });
    }
    onClear();
  };

  const stageAll = () => {
    for (const id of ids) {
      stage.mutate(id);
    }
    onClear();
  };

  const deleteAll = () => {
    for (const id of ids) {
      del.mutate(id);
    }
    setConfirmDelete(false);
    onClear();
  };

  return (
    <section
      aria-label="Bulk actions"
      className="sticky bottom-4 z-20 mx-auto flex w-fit items-center gap-2 rounded-full bg-popover px-3 py-2 text-sm shadow-lg ring-1 ring-foreground/10"
    >
      <span className="text-muted-foreground">{ids.length} selected</span>
      <Button size="sm" variant="secondary" onClick={markDone}>
        Mark done
      </Button>
      <Button size="sm" variant="secondary" onClick={stageAll}>
        Stage selected
      </Button>
      <Button size="sm" variant="destructive" onClick={() => setConfirmDelete(true)}>
        Delete selected
      </Button>
      <Button size="sm" variant="ghost" onClick={onClear}>
        Clear
      </Button>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {ids.length} task(s)?</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            This cannot be undone. Tasks will be permanently removed.
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={deleteAll}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
