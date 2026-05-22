/**
 * Schedule picker used inside the script editor form. Reads/writes through
 * the parent's react-hook-form context (`useFormContext`) so its zod
 * resolver controls validation.
 *
 * Form shape it expects on the parent:
 * ```ts
 * {
 *   schedule:
 *     | { kind: 'every_tick' }
 *     | { kind: 'daily' }
 *     | { kind: 'weekly'; weekday: Weekday }
 *     | { kind: 'monthly'; day: number | 'last' };
 *   confirm_every_tick?: boolean;
 * }
 * ```
 *
 * Switching `kind` resets the dependent field so we never carry stale
 * `weekday` / `day` values into a payload that no longer needs them.
 *
 * When `kind === 'every_tick'` we render the yellow inline warning from
 * spec §6 and a confirm checkbox. The parent's zod schema MUST refine
 * `confirm_every_tick === true` when the kind is `every_tick` — see
 * `editor-page.tsx` for the canonical schema.
 */

import { AlertTriangleIcon } from 'lucide-react';
import { Controller, useFormContext, useWatch } from 'react-hook-form';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { MonthlyDay, ScheduleKind, Weekday } from '@/types/script';

const WEEKDAYS: { value: Weekday; label: string }[] = [
  { value: 'monday', label: 'Monday' },
  { value: 'tuesday', label: 'Tuesday' },
  { value: 'wednesday', label: 'Wednesday' },
  { value: 'thursday', label: 'Thursday' },
  { value: 'friday', label: 'Friday' },
  { value: 'saturday', label: 'Saturday' },
  { value: 'sunday', label: 'Sunday' },
];

const KINDS: { value: ScheduleKind; label: string }[] = [
  { value: 'every_tick', label: 'Every tick (15 min)' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekly', label: 'Weekly' },
  { value: 'monthly', label: 'Monthly' },
];

/** Build the default schedule object for a given kind. */
export function defaultScheduleFor(
  kind: ScheduleKind,
):
  | { kind: 'every_tick' }
  | { kind: 'daily' }
  | { kind: 'weekly'; weekday: Weekday }
  | { kind: 'monthly'; day: MonthlyDay } {
  switch (kind) {
    case 'every_tick':
      return { kind: 'every_tick' };
    case 'daily':
      return { kind: 'daily' };
    case 'weekly':
      return { kind: 'weekly', weekday: 'monday' };
    case 'monthly':
      return { kind: 'monthly', day: 1 };
  }
}

export function ScheduleSubForm() {
  const { control, setValue, getValues, formState } = useFormContext();
  // useWatch keeps the render in sync with the current `schedule.kind` without
  // re-rendering the parent.
  const kind = useWatch({ control, name: 'schedule.kind' }) as ScheduleKind | undefined;
  const errors = formState.errors as Record<string, { message?: string } | undefined>;

  const onKindChange = (next: ScheduleKind) => {
    const prev = (getValues('schedule') ?? {}) as { kind?: ScheduleKind };
    if (prev.kind === next) return;
    // shouldDirty so the parent's isDirty / unsaved-guard fires on changes.
    setValue('schedule', defaultScheduleFor(next), {
      shouldDirty: true,
      shouldValidate: true,
    });
    if (next !== 'every_tick') {
      setValue('confirm_every_tick', false, { shouldDirty: true });
    }
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="schedule-kind">Schedule</Label>
        <Select value={kind ?? 'daily'} onValueChange={(v) => onKindChange(v as ScheduleKind)}>
          <SelectTrigger id="schedule-kind" className="w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {KINDS.map((k) => (
              <SelectItem key={k.value} value={k.value}>
                {k.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {kind === 'weekly' && (
        <Controller
          control={control}
          name="schedule.weekday"
          render={({ field }) => (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="schedule-weekday">Weekday</Label>
              <Select value={field.value as Weekday} onValueChange={field.onChange}>
                <SelectTrigger id="schedule-weekday" className="w-56">
                  <SelectValue placeholder="Pick a weekday" />
                </SelectTrigger>
                <SelectContent>
                  {WEEKDAYS.map((w) => (
                    <SelectItem key={w.value} value={w.value}>
                      {w.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        />
      )}

      {kind === 'monthly' && (
        <Controller
          control={control}
          name="schedule.day"
          render={({ field }) => {
            const stringified = field.value === 'last' ? 'last' : String(field.value ?? 1);
            return (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="schedule-day">Day of month</Label>
                <Select
                  value={stringified}
                  onValueChange={(v) =>
                    field.onChange(v === 'last' ? 'last' : Number.parseInt(v, 10))
                  }
                >
                  <SelectTrigger id="schedule-day" className="w-56">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Array.from({ length: 31 }, (_, i) => i + 1).map((n) => (
                      <SelectItem key={n} value={String(n)}>
                        {n}
                      </SelectItem>
                    ))}
                    <SelectItem value="last">Last day of month</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            );
          }}
        />
      )}

      {kind === 'every_tick' && (
        <div
          data-testid="every-tick-banner"
          className="flex flex-col gap-2 rounded-md border border-yellow-500/40 bg-yellow-500/10 p-3 text-sm"
        >
          <div className="flex items-center gap-2 font-medium">
            <AlertTriangleIcon className="size-4 text-yellow-600 dark:text-yellow-400" />
            Every-tick scheduling
          </div>
          <p className="text-muted-foreground">
            Every-tick scripts run on every global tick (currently every 15 min). Buggy scripts can
            flood your task list. Confirm to use this schedule.
          </p>
          <Controller
            control={control}
            name="confirm_every_tick"
            render={({ field }) => (
              <div className="flex items-center gap-2">
                <Checkbox
                  id="confirm-every-tick"
                  checked={field.value === true}
                  onCheckedChange={(c) => field.onChange(c === true)}
                  aria-label="Confirm every-tick scheduling"
                />
                <Label htmlFor="confirm-every-tick" className="font-normal">
                  I understand
                </Label>
              </div>
            )}
          />
          {errors.confirm_every_tick?.message && (
            <p className="text-xs text-destructive" role="alert">
              {errors.confirm_every_tick.message}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
