/**
 * Status pill for `RunStatus`. Used in the runs list and detail header.
 *
 * Shadcn's built-in Badge variants don't include an orange/warning color, so
 * timeout uses an inline className. The pill always pairs a lucide icon with
 * the label so the status is still distinguishable when color contrast is low.
 */

import { CheckIcon, ClockIcon, Loader2Icon, XIcon } from 'lucide-react';
import type { ComponentProps } from 'react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import type { RunStatus } from '@/types/run';

export interface StatusPillProps extends Omit<ComponentProps<typeof Badge>, 'variant'> {
  status: RunStatus;
}

interface StatusConfig {
  label: string;
  variant: ComponentProps<typeof Badge>['variant'];
  className?: string;
  Icon: typeof CheckIcon;
  spin?: boolean;
}

const STATUS_CONFIG: Record<RunStatus, StatusConfig> = {
  running: {
    label: 'running',
    variant: 'default',
    Icon: Loader2Icon,
    spin: true,
  },
  ok: {
    label: 'ok',
    variant: 'secondary',
    className: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-900/40 dark:text-emerald-200',
    Icon: CheckIcon,
  },
  error: {
    label: 'error',
    variant: 'destructive',
    Icon: XIcon,
  },
  timeout: {
    label: 'timeout',
    variant: 'outline',
    // No built-in orange variant — opt into Tailwind classes here.
    className:
      'border-orange-300 bg-orange-100 text-orange-900 dark:border-orange-800 dark:bg-orange-900/40 dark:text-orange-200',
    Icon: ClockIcon,
  },
};

export function StatusPill({ status, className, ...rest }: StatusPillProps) {
  const cfg = STATUS_CONFIG[status];
  return (
    <Badge
      variant={cfg.variant}
      className={cn(cfg.className, className)}
      data-status={status}
      {...rest}
    >
      <cfg.Icon className={cn(cfg.spin && 'animate-spin')} />
      {cfg.label}
    </Badge>
  );
}
