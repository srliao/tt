import { zodResolver } from '@hookform/resolvers/zod';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { FormProvider, type UseFormReturn, useForm } from 'react-hook-form';
import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import { defaultScheduleFor, ScheduleSubForm } from './schedule-sub-form';

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

const formSchema = z
  .object({
    schedule: scheduleSchema,
    confirm_every_tick: z.boolean().optional(),
  })
  .refine((data) => data.schedule.kind !== 'every_tick' || data.confirm_every_tick === true, {
    message: 'Please confirm every_tick scheduling',
    path: ['confirm_every_tick'],
  });

type FormValues = z.infer<typeof formSchema>;

interface HarnessProps {
  defaultValues: FormValues;
  onSubmit?: (values: FormValues) => void;
  onReady?: (form: UseFormReturn<FormValues>) => void;
}

function Harness({ defaultValues, onSubmit, onReady }: HarnessProps) {
  const methods = useForm<FormValues>({
    resolver: zodResolver(formSchema) as never,
    defaultValues,
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: report once
  useEffect(() => {
    onReady?.(methods);
  }, []);
  const submit = methods.handleSubmit((values) => onSubmit?.(values));
  return (
    <FormProvider {...methods}>
      <form onSubmit={submit}>
        <ScheduleSubForm />
        <button type="submit">submit</button>
      </form>
    </FormProvider>
  );
}

describe('defaultScheduleFor', () => {
  it('returns a sensible default for each kind', () => {
    expect(defaultScheduleFor('every_tick')).toEqual({ kind: 'every_tick' });
    expect(defaultScheduleFor('daily')).toEqual({ kind: 'daily' });
    expect(defaultScheduleFor('weekly')).toEqual({ kind: 'weekly', weekday: 'monday' });
    expect(defaultScheduleFor('monthly')).toEqual({ kind: 'monthly', day: 1 });
  });
});

describe('ScheduleSubForm', () => {
  it('shows the yellow banner when kind is every_tick', () => {
    render(<Harness defaultValues={{ schedule: { kind: 'every_tick' } }} />);
    expect(screen.getByTestId('every-tick-banner')).toBeTruthy();
    expect(screen.getByText(/Every-tick scripts run on every global tick/)).toBeTruthy();
  });

  it('blocks submission of every_tick without the confirm checkbox', async () => {
    let captured: FormValues | undefined;
    render(
      <Harness
        defaultValues={{ schedule: { kind: 'every_tick' } }}
        onSubmit={(v) => {
          captured = v;
        }}
      />,
    );
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'submit' }));
    });
    expect(captured).toBeUndefined();
    expect(await screen.findByText(/Please confirm every_tick scheduling/)).toBeTruthy();
  });

  it('allows submission of every_tick once the box is checked', async () => {
    let captured: FormValues | undefined;
    render(
      <Harness
        defaultValues={{ schedule: { kind: 'every_tick' }, confirm_every_tick: false }}
        onSubmit={(v) => {
          captured = v;
        }}
      />,
    );
    await act(async () => {
      fireEvent.click(screen.getByRole('checkbox', { name: /Confirm every-tick scheduling/i }));
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'submit' }));
    });
    expect(captured).toEqual({
      schedule: { kind: 'every_tick' },
      confirm_every_tick: true,
    });
  });

  it('shows the weekday picker for weekly, then swaps to a day picker for monthly', async () => {
    let form: UseFormReturn<FormValues> | undefined;
    render(
      <Harness
        defaultValues={{ schedule: { kind: 'weekly', weekday: 'monday' } }}
        onReady={(f) => {
          form = f;
        }}
      />,
    );
    expect(screen.getByLabelText('Weekday')).toBeTruthy();

    // Radix Select is portalled and tricky to drive in jsdom; instead, drive
    // the underlying form value directly. The kind-switch handler in the
    // component shape mirrors what `onKindChange` does internally (replaces
    // the entire `schedule` value with the new kind's default).
    await act(async () => {
      form?.setValue('schedule', { kind: 'monthly', day: 1 }, { shouldDirty: true });
    });
    expect(screen.queryByLabelText('Weekday')).toBeNull();
    expect(screen.getByLabelText('Day of month')).toBeTruthy();
  });
});
