/**
 * The /stage page. Wires together:
 *
 * - The top bar with "N staged" count + Clear-finished + Clear-stage + "Add
 *   from list →" actions.
 * - `<AddTaskModal stageAfterCreate>` — opened by the global `n` shortcut so
 *   the new task is staged immediately.
 * - The `<SoftCapHint>` banner when count > 7.
 * - The `<StageList>` (dnd-kit reorder + per-row state cycle + unstage).
 * - Per-page j/k/Enter/e/u/space/d shortcuts, scoped to the list container.
 * - `<EditTaskModal>` mounted at the bottom — opened either by clicking a
 *   row title or pressing `e`/`Enter` on a focused row.
 */

import { Link, useNavigate } from '@tanstack/react-router';
import { ArrowRightIcon, ListTodoIcon, Trash2Icon } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useClearFinishedFromStage, useClearStage, useStagedTasks } from '@/api/stage';
import { useSetTaskState, useUnstageTask } from '@/api/tasks';
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
import type { Task } from '@/types/task';
import { AddTaskModal, useNewTaskListener } from '../tasks/add-task-modal';
import { EditTaskModal } from '../tasks/edit-task-modal';
import { SoftCapHint } from './soft-cap-hint';
import { StageList } from './stage-list';
import { nextState } from './stage-row';

export function StagePage() {
  const { data: tasks = [], isLoading } = useStagedTasks();
  const clearAll = useClearStage();
  const clearFinished = useClearFinishedFromStage();
  const setState = useSetTaskState();
  const unstage = useUnstageTask();

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [confirmClearOpen, setConfirmClearOpen] = useState(false);
  const [focusedId, setFocusedId] = useState<number | null>(null);

  useNewTaskListener(() => setCreating(true));

  const containerRef = useRef<HTMLDivElement>(null);
  useStageShortcuts({
    containerRef,
    tasks,
    focusedId,
    setFocusedId,
    onEdit: (t) => setEditing(t),
    onCycleState: (id, st) => setState.mutate({ id, state: st }),
    onUnstage: (id) => unstage.mutate(id),
  });

  const showEmpty = !isLoading && tasks.length === 0;

  return (
    <section className="flex min-h-[calc(100vh-3.5rem)] flex-col px-4 py-4">
      <header className="mb-3 flex items-center justify-between gap-2">
        <div className="flex items-center gap-3">
          <h1 className="font-heading text-xl font-medium">Stage</h1>
          <span className="text-sm text-muted-foreground" data-testid="stage-count">
            {tasks.length} staged
          </span>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => clearFinished.mutate()}
            disabled={tasks.length === 0}
          >
            Clear finished
          </Button>
          <AlertDialog open={confirmClearOpen} onOpenChange={setConfirmClearOpen}>
            <AlertDialogTrigger asChild>
              <Button
                variant="destructive"
                size="sm"
                disabled={tasks.length === 0}
                aria-label="Clear stage"
              >
                <Trash2Icon /> Clear stage
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Clear the entire stage?</AlertDialogTitle>
                <AlertDialogDescription>
                  Every staged task will be unstaged. Tasks themselves are not deleted; you can
                  re-stage them from the tasks list.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  onClick={() => {
                    clearAll.mutate();
                    setConfirmClearOpen(false);
                  }}
                >
                  Clear stage
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
          <Button asChild size="sm" variant="secondary">
            <Link to="/tasks">
              Add from list <ArrowRightIcon />
            </Link>
          </Button>
        </div>
      </header>

      <div
        ref={containerRef}
        tabIndex={-1}
        data-stage-container
        className="flex flex-col gap-3 focus-visible:outline-none"
      >
        <SoftCapHint count={tasks.length} />

        {showEmpty ? (
          <EmptyState />
        ) : (
          <StageList tasks={tasks} focusedId={focusedId} onEdit={(t) => setEditing(t)} />
        )}
      </div>

      <AddTaskModal open={creating} onOpenChange={setCreating} stageAfterCreate />

      <EditTaskModal
        task={editing}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
      />
    </section>
  );
}

function EmptyState() {
  const navigate = useNavigate();
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-12 text-center">
      <ListTodoIcon className="size-8 text-muted-foreground" aria-hidden="true" />
      <h2 className="font-heading text-lg">Nothing staged</h2>
      <p className="max-w-md text-sm text-muted-foreground">
        Pick a few tasks from your list to focus on now.
      </p>
      <Button onClick={() => void navigate({ to: '/tasks' })}>Go to tasks</Button>
    </div>
  );
}

interface StageShortcutsArgs {
  containerRef: React.RefObject<HTMLDivElement | null>;
  tasks: Task[];
  focusedId: number | null;
  setFocusedId: (id: number | null) => void;
  onEdit: (task: Task) => void;
  onCycleState: (id: number, state: ReturnType<typeof nextState>) => void;
  onUnstage: (id: number) => void;
}

/**
 * j/k row navigation + enter/e/u/space/d action keys, scoped to the stage
 * container. Mirrors `useTableShortcuts` from the /tasks table so the two
 * pages behave consistently.
 *
 * - `j`/`k` — move focus down/up.
 * - `Enter`/`e` — open the edit modal for the focused row.
 * - `u` — unstage the focused row.
 * - `d` — cycle the focused row's state (not_done → done → cancelled → ...).
 * - `space` — also cycles state (matches the spec's "space toggles done").
 */
function useStageShortcuts({
  containerRef,
  tasks,
  focusedId,
  setFocusedId,
  onEdit,
  onCycleState,
  onUnstage,
}: StageShortcutsArgs) {
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const handler = (event: KeyboardEvent) => {
      if (
        !containerRef.current?.contains(event.target as Node) &&
        event.target !== containerRef.current
      ) {
        return;
      }
      const target = event.target as HTMLElement | null;
      if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
        return;
      }
      if (tasks.length === 0) return;

      const currentIdx = focusedId != null ? tasks.findIndex((t) => t.id === focusedId) : -1;

      const setIdx = (idx: number) => {
        const clamped = Math.max(0, Math.min(tasks.length - 1, idx));
        setFocusedId(tasks[clamped].id);
      };

      if (event.key === 'j') {
        event.preventDefault();
        setIdx(currentIdx === -1 ? 0 : currentIdx + 1);
        return;
      }
      if (event.key === 'k') {
        event.preventDefault();
        setIdx(currentIdx === -1 ? 0 : currentIdx - 1);
        return;
      }
      if (focusedId == null) return;
      const focused = tasks.find((t) => t.id === focusedId);
      if (!focused) return;

      if (event.key === 'Enter' || event.key === 'e') {
        event.preventDefault();
        onEdit(focused);
      } else if (event.key === 'u') {
        event.preventDefault();
        onUnstage(focused.id);
      } else if (event.key === 'd' || event.key === ' ') {
        event.preventDefault();
        onCycleState(focused.id, nextState(focused.state));
      }
    };

    el.addEventListener('keydown', handler);
    return () => el.removeEventListener('keydown', handler);
  }, [containerRef, tasks, focusedId, setFocusedId, onEdit, onCycleState, onUnstage]);
}
