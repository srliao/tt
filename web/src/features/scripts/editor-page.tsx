/**
 * The script editor page. Used for both `/scripts/$id` (edit existing) and
 * `/scripts/new` (create) — `id` is omitted for create-mode. Layout:
 *
 *   ┌──────────────────────────────────────────────┐
 *   │ Name input        [enabled switch] [Run now] │
 *   │ Schedule sub-form                            │
 *   │ Code editor                                  │
 *   │ [Delete]                              [Save] │
 *   └──────────────────────────────────────────────┘
 *                                ┌────────────────┐
 *                                │ Tabs: API|Tags │
 *                                │  Spawned|Runs  │
 *                                └────────────────┘
 *
 * Unsaved-changes guard: TanStack Router's `useBlocker({shouldBlockFn})`
 * fires when `form.formState.isDirty` is true. The harness still attaches a
 * `beforeunload` listener for browser-level navigation.
 *
 * "Run now" is intentionally disabled in two cases:
 *  1. The form is dirty — running stale code is confusing; force a save first.
 *  2. The script is disabled — backend would 409 anyway.
 *
 * While a save is in flight a full-section overlay blocks pointer events so
 * the user can't double-submit, delete, or navigate via the action bar.
 *
 * `Cmd/Ctrl-Enter` from anywhere inside the form submits it.
 */

import { zodResolver } from '@hookform/resolvers/zod';
import { useBlocker, useNavigate } from '@tanstack/react-router';
import { Loader2Icon, PlayIcon, Trash2Icon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Controller, FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import {
  useCreateScript,
  useDeleteScript,
  useRunScript,
  useScript,
  useUpdateScript,
} from '@/api/scripts';
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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { ApiError } from '@/lib/api';
import type { Schedule, Script, ScriptCreateInput } from '@/types/script';
import { CheatsheetApi } from './cheatsheet-api';
import { CheatsheetTags } from './cheatsheet-tags';
import { CodeEditor } from './code-editor';
import { RecentRunsTable } from './recent-runs-table';
import { ScheduleSubForm } from './schedule-sub-form';
import { SpawnedTasksPanel } from './spawned-tasks-panel';

const scheduleSchema = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('every_tick') }),
  z.object({ kind: z.literal('daily') }),
  z.object({
    kind: z.literal('weekly'),
    weekday: z.enum(['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday']),
  }),
  z.object({
    kind: z.literal('monthly'),
    day: z.union([z.number().int().min(1).max(31), z.literal('last')]),
  }),
]);

export const editorFormSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    code: z.string(),
    enabled: z.boolean(),
    schedule: scheduleSchema,
    confirm_every_tick: z.boolean().optional(),
  })
  .refine((data) => data.schedule.kind !== 'every_tick' || data.confirm_every_tick === true, {
    message: 'Please confirm every_tick scheduling',
    path: ['confirm_every_tick'],
  });

export type EditorFormValues = z.infer<typeof editorFormSchema>;

const NEW_SCRIPT_TEMPLATE = `(function () {
  const new_task = { title: "" };

  ctx.queueTask(new_task);
})();
`;

function defaultValuesFor(script: Script | undefined): EditorFormValues {
  if (!script) {
    return {
      name: '',
      code: NEW_SCRIPT_TEMPLATE,
      enabled: true,
      schedule: { kind: 'daily' },
      confirm_every_tick: false,
    };
  }
  return {
    name: script.name,
    code: script.code,
    enabled: script.enabled,
    schedule: script.schedule,
    confirm_every_tick: script.schedule.kind === 'every_tick',
  };
}

export interface ScriptEditorPageProps {
  /** When undefined, the page is in create-mode (`/scripts/new`). */
  id?: number;
}

export function ScriptEditorPage({ id }: ScriptEditorPageProps) {
  const navigate = useNavigate();
  const isEdit = id !== undefined;
  const { data: script } = useScript(isEdit ? id : undefined);
  const createScript = useCreateScript();
  const updateScript = useUpdateScript();
  const deleteScript = useDeleteScript();
  const runScript = useRunScript(isEdit ? (id as number) : -1);

  const [serverError, setServerError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const methods = useForm<EditorFormValues>({
    resolver: zodResolver(editorFormSchema) as never,
    defaultValues: defaultValuesFor(undefined),
  });

  // Rehydrate the form once the script loads (edit mode). Guard on
  // `schedule` because the server response is unconditionally typed but
  // a stale/empty cache hit during navigation could yield a partial obj.
  useEffect(() => {
    if (script && typeof script === 'object' && 'schedule' in script && script.schedule) {
      methods.reset(defaultValuesFor(script));
    }
  }, [script, methods]);

  const enabled = methods.watch('enabled');
  const isDirty = methods.formState.isDirty;

  // In-app navigation blocker (TanStack Router).
  useBlocker({
    shouldBlockFn: () => {
      if (!isDirty) return false;
      // Synchronous prompt — TanStack accepts a Promise<boolean> too but the
      // browser's confirm() is good enough for v1.
      return !window.confirm('You have unsaved changes — leave anyway?');
    },
    enableBeforeUnload: () => isDirty,
  });

  // Browser-level guard (Cmd-W, refresh, etc.).
  useEffect(() => {
    if (!isDirty) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [isDirty]);

  const onSubmit = methods.handleSubmit(async (values) => {
    setServerError(null);
    const payload: ScriptCreateInput = {
      name: values.name.trim(),
      code: values.code,
      enabled: values.enabled,
      schedule: values.schedule as Schedule,
    };
    try {
      if (isEdit && id !== undefined) {
        await updateScript.mutateAsync({ id, input: payload });
        methods.reset(values);
      } else {
        const created = await createScript.mutateAsync(payload);
        // Reset before navigating so the blocker doesn't fire.
        methods.reset(defaultValuesFor(created));
        void navigate({ to: '/scripts/$id', params: { id: String(created.id) } });
      }
    } catch (err) {
      if (err instanceof ApiError) {
        setServerError(err.message);
      } else {
        throw err;
      }
    }
  });

  const onKeyDown = (event: React.KeyboardEvent<HTMLFormElement>) => {
    if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
      event.preventDefault();
      void onSubmit();
    }
  };

  const onRunNow = () => {
    if (!isEdit || id === undefined) return;
    runScript.mutate(undefined, {
      onSuccess: (data) => {
        void navigate({ to: '/runs/$id', params: { id: String(data.run_id) } });
      },
    });
  };

  const onDelete = () => {
    if (!isEdit || id === undefined) return;
    deleteScript.mutate(id, {
      onSuccess: () => {
        // Skip the dirty guard for the post-delete navigation.
        methods.reset(defaultValuesFor(undefined));
        void navigate({ to: '/scripts' });
      },
    });
  };

  const runDisabled = !isEdit || isDirty || !enabled || runScript.isPending;
  const isSaving = methods.formState.isSubmitting;

  return (
    <FormProvider {...methods}>
      <section className="relative mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-4 lg:h-[calc(100vh-3.5rem)] lg:flex-row lg:overflow-hidden">
        <form
          className="flex min-w-0 flex-1 flex-col gap-4 lg:min-h-0"
          onSubmit={onSubmit}
          onKeyDown={onKeyDown}
        >
          <header className="flex flex-wrap items-end gap-3">
            <div className="flex flex-1 flex-col gap-1.5">
              <Label htmlFor="script-title-field">Name</Label>
              {/*
                Driven via Controller (not `register`) so we can override the
                HTML `name` attribute. Password managers (1Password, LastPass,
                Bitwarden) heuristically autofill inputs literally named
                "name" — the data-* attributes plus a non-suggestive name
                opt out across all the major ones.
              */}
              <Controller
                control={methods.control}
                name="name"
                render={({ field }) => (
                  <Input
                    id="script-title-field"
                    name="script-title-field"
                    autoFocus
                    autoComplete="off"
                    data-1p-ignore
                    data-lpignore="true"
                    data-form-type="other"
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    ref={field.ref}
                    aria-invalid={!!methods.formState.errors.name}
                  />
                )}
              />
              {methods.formState.errors.name && (
                <p className="text-xs text-destructive" role="alert">
                  {methods.formState.errors.name.message}
                </p>
              )}
            </div>
            <Controller
              control={methods.control}
              name="enabled"
              render={({ field }) => (
                <div className="flex items-center gap-2 pb-2">
                  <Switch
                    id="script-enabled"
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                  <Label htmlFor="script-enabled" className="font-normal">
                    Enabled
                  </Label>
                </div>
              )}
            />
            {isEdit && (
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={runDisabled}
                onClick={onRunNow}
                title={
                  isDirty
                    ? 'Save changes first'
                    : !enabled
                      ? 'Enable the script first'
                      : 'Run this script now'
                }
              >
                <PlayIcon /> Run now
              </Button>
            )}
          </header>

          {serverError && (
            <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {serverError}
            </p>
          )}

          <ScheduleSubForm />

          <Controller
            control={methods.control}
            name="code"
            render={({ field }) => (
              <div className="flex flex-col gap-1.5 lg:min-h-0 lg:flex-1">
                <Label>Code</Label>
                <div className="lg:min-h-0 lg:flex-1">
                  <CodeEditor value={field.value} onChange={field.onChange} />
                </div>
                <p className="text-xs text-muted-foreground">
                  Cmd/Ctrl-Enter saves. Tip: paste tag names from the Tags sidebar.
                </p>
              </div>
            )}
          />

          <div className="flex flex-wrap items-center justify-between gap-3">
            {isEdit ? (
              <Button
                type="button"
                variant="destructive"
                size="sm"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2Icon /> Delete
              </Button>
            ) : (
              <span />
            )}
            <Button type="submit" size="sm" disabled={isSaving || (isEdit && !isDirty)}>
              {isEdit ? 'Save changes' : 'Create script'}
            </Button>
          </div>
        </form>

        <aside className="flex w-full shrink-0 flex-col gap-2 lg:w-80 lg:min-h-0 lg:overflow-y-auto">
          <Tabs defaultValue="api">
            <TabsList className="w-full">
              <TabsTrigger value="api">API</TabsTrigger>
              <TabsTrigger value="tags">Tags</TabsTrigger>
              {isEdit && <TabsTrigger value="spawned">Spawned</TabsTrigger>}
              {isEdit && <TabsTrigger value="runs">Runs</TabsTrigger>}
            </TabsList>
            <TabsContent value="api">
              <CheatsheetApi />
            </TabsContent>
            <TabsContent value="tags">
              <CheatsheetTags />
            </TabsContent>
            {isEdit && id !== undefined && (
              <>
                <TabsContent value="spawned">
                  <SpawnedTasksPanel scriptId={id} />
                </TabsContent>
                <TabsContent value="runs">
                  <RecentRunsTable scriptId={id} />
                </TabsContent>
              </>
            )}
          </Tabs>
        </aside>

        {isSaving && (
          <div
            className="absolute inset-0 z-50 flex items-center justify-center rounded-md bg-background/60 backdrop-blur-sm"
            role="status"
            aria-live="polite"
            aria-label="Saving"
          >
            <div className="flex items-center gap-2 rounded-md border bg-background px-4 py-3 shadow-md">
              <Loader2Icon className="size-4 animate-spin" />
              <span className="text-sm font-medium">Saving…</span>
            </div>
          </div>
        )}
      </section>

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this script?</AlertDialogTitle>
            <AlertDialogDescription>
              The script and its run history will be removed. Tasks it has already spawned are kept.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                setConfirmDelete(false);
                onDelete();
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </FormProvider>
  );
}
