/**
 * The /scripts page. Lists all userscripts as a small table with affordances
 * for toggling `enabled`, running on-demand, and deleting. Clicking the name
 * navigates to the editor.
 *
 * Empty state per spec §6: "Userscripts let you auto-create tasks on a
 * schedule. Examples: weekly review, monthly bills, after-N-days follow-ups."
 *
 * TODO: optionally pre-seed one disabled example script — skipped for v1.
 */

import { Link, useNavigate } from '@tanstack/react-router';
import { formatDistanceToNow, parseISO } from 'date-fns';
import { MoreHorizontalIcon, PlayIcon, PlusIcon, Trash2Icon } from 'lucide-react';
import { useState } from 'react';
import { useDeleteScript, useRunScript, useScripts, useUpdateScript } from '@/api/scripts';
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
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import type { Schedule, Script } from '@/types/script';

/** Human-friendly summary of a `Schedule`. Exported for tests. */
export function humanizeSchedule(schedule: Schedule): string {
  switch (schedule.kind) {
    case 'every_tick':
      return 'Every 15 min';
    case 'daily':
      return 'Daily';
    case 'weekly':
      return `Weekly on ${capitalize(schedule.weekday).slice(0, 3)}`;
    case 'monthly':
      return schedule.day === 'last' ? 'Monthly last day' : `Monthly day ${schedule.day}`;
  }
}

function capitalize(s: string): string {
  return s.length === 0 ? s : s[0].toUpperCase() + s.slice(1);
}

export function ScriptsListPage() {
  const { data: scripts = [], isLoading } = useScripts();

  const showEmpty = !isLoading && scripts.length === 0;
  return (
    <section className="mx-auto flex w-full max-w-4xl flex-col gap-4 px-4 py-4">
      <header className="flex items-center justify-between">
        <h1 className="font-heading text-xl font-medium">Scripts</h1>
        <Button asChild size="sm">
          <Link to="/scripts/new">
            <PlusIcon /> New script
          </Link>
        </Button>
      </header>

      {showEmpty ? (
        <EmptyState />
      ) : (
        <table aria-label="Scripts" className="w-full text-sm">
          <thead className="text-xs text-muted-foreground">
            <tr className="border-b">
              <th className="px-2 py-2 text-left font-medium">Name</th>
              <th className="w-40 px-2 py-2 text-left font-medium">Schedule</th>
              <th className="w-20 px-2 py-2 text-left font-medium">Enabled</th>
              <th className="w-40 px-2 py-2 text-left font-medium">Last run</th>
              <th className="w-8 px-1 py-2" />
            </tr>
          </thead>
          <tbody>
            {scripts.map((script) => (
              <ScriptRow key={script.id} script={script} />
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-12 text-center">
      <h2 className="font-heading text-lg">No scripts yet</h2>
      <p className="max-w-md text-sm text-muted-foreground">
        Userscripts let you auto-create tasks on a schedule. Examples: weekly review, monthly bills,
        after-N-days follow-ups.
      </p>
      <Button asChild>
        <Link to="/scripts/new">
          <PlusIcon /> Create your first script
        </Link>
      </Button>
    </div>
  );
}

function ScriptRow({ script }: { script: Script }) {
  const navigate = useNavigate();
  const update = useUpdateScript();
  const run = useRunScript(script.id);
  const del = useDeleteScript();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const onToggleEnabled = (next: boolean) => {
    update.mutate({
      id: script.id,
      input: {
        name: script.name,
        code: script.code,
        enabled: next,
        schedule: script.schedule,
      },
    });
  };

  const onRunNow = () => {
    run.mutate(undefined, {
      onSuccess: (data) => {
        void navigate({ to: '/runs/$id', params: { id: String(data.run_id) } });
      },
    });
  };

  return (
    <tr data-script-id={script.id} className="border-b hover:bg-muted/40">
      <td className="px-2 py-2 align-middle">
        <Link
          to="/scripts/$id"
          params={{ id: String(script.id) }}
          className="text-left text-sm font-medium hover:underline"
        >
          {script.name}
        </Link>
      </td>
      <td className="px-2 py-2 align-middle">
        <span className="text-xs text-muted-foreground">{humanizeSchedule(script.schedule)}</span>
      </td>
      <td className="px-2 py-2 align-middle">
        <Checkbox
          checked={script.enabled}
          onCheckedChange={(c) => onToggleEnabled(c === true)}
          aria-label={`Enabled for ${script.name}`}
        />
      </td>
      <td className="px-2 py-2 align-middle">
        {script.last_run_at ? (
          <span className="text-xs text-muted-foreground">
            {formatDistanceToNow(parseISO(script.last_run_at), { addSuffix: true })}
          </span>
        ) : (
          <Badge variant="outline" className="text-[10px]">
            never
          </Badge>
        )}
      </td>
      <td className="px-1 py-2 align-middle">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${script.name}`}>
              <MoreHorizontalIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem disabled={!script.enabled || run.isPending} onClick={onRunNow}>
              <PlayIcon /> Run now
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={() => setConfirmOpen(true)}>
              <Trash2Icon /> Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete script "{script.name}"?</AlertDialogTitle>
              <AlertDialogDescription>
                This removes the script and its run history. Tasks it has already spawned are kept.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                onClick={() => {
                  del.mutate(script.id);
                  setConfirmOpen(false);
                }}
              >
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </td>
    </tr>
  );
}
