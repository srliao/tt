import { XIcon } from 'lucide-react';
import type * as React from 'react';

import { useTheme } from '@/components/theme-provider';
import { tagColor, tagColorDark } from '@/lib/tag-color';
import { cn } from '@/lib/utils';

/**
 * <TagChip> — the canonical pill used to render a tag anywhere in the UI.
 *
 * Purely presentational. Filter / palette / combobox wiring lives in the
 * caller; this component only renders a colored pill, an optional remove
 * button, and forwards clicks if asked. Color selection is delegated to
 * `tag-color.ts` so every chip with the same name shares one hue.
 */

export interface TagChipProps {
  name: string;
  /** When set, renders an X button that fires onRemove. */
  onRemove?: () => void;
  /** When set, the chip is a button — clicking fires onClick. */
  onClick?: (event: React.MouseEvent) => void;
  /** Render with a transparent background + neutral text. */
  variant?: 'solid' | 'outline';
  /** Visually dim — for excluded filters etc. */
  dim?: boolean;
  size?: 'sm' | 'md';
  /** Extra classes for the outer element. */
  className?: string;
  /** Optional aria-label override for accessibility. */
  ariaLabel?: string;
}

export function TagChip({
  name,
  onRemove,
  onClick,
  variant = 'solid',
  dim,
  size = 'sm',
  className,
  ariaLabel,
}: TagChipProps) {
  const { resolvedTheme } = useTheme();
  const palette = resolvedTheme === 'dark' ? tagColorDark(name) : tagColor(name);

  // Solid chips fill the background with the tag's hue. Outline chips drop the
  // fill and use a neutral border so the dot is the sole color cue — useful in
  // dense lists where many chips might cluster.
  const style: React.CSSProperties =
    variant === 'solid' ? { backgroundColor: palette.bg, color: palette.fg } : { color: 'inherit' };

  const sizeClasses = size === 'md' ? 'h-6 px-2 text-sm gap-1.5' : 'h-5 px-1.5 text-xs gap-1';

  const baseClasses = cn(
    'inline-flex w-fit shrink-0 items-center rounded-full font-medium whitespace-nowrap select-none',
    'border transition-colors',
    variant === 'solid' ? 'border-transparent' : 'border-border bg-transparent text-foreground',
    sizeClasses,
    dim && 'opacity-50',
    onClick &&
      'cursor-pointer hover:brightness-95 focus-visible:outline-2 focus-visible:outline-ring focus-visible:outline-offset-1',
    className,
  );

  const dot = (
    <span
      aria-hidden
      className={cn(
        'inline-block shrink-0 rounded-full',
        size === 'md' ? 'h-2 w-2' : 'h-1.5 w-1.5',
      )}
      style={{ backgroundColor: palette.dot }}
    />
  );

  const label = (
    <span className="truncate" data-slot="tag-chip-label">
      {name}
    </span>
  );

  const remove = onRemove ? (
    <button
      type="button"
      aria-label={`Remove ${name}`}
      onClick={(e) => {
        e.stopPropagation();
        onRemove();
      }}
      className={cn(
        'inline-flex items-center justify-center rounded-full -mr-0.5',
        'hover:bg-black/10 dark:hover:bg-white/15',
        'focus-visible:outline-2 focus-visible:outline-ring focus-visible:outline-offset-1',
        size === 'md' ? 'h-4 w-4' : 'h-3.5 w-3.5',
      )}
    >
      <XIcon className={size === 'md' ? 'h-3 w-3' : 'h-2.5 w-2.5'} />
    </button>
  ) : null;

  const children = (
    <>
      {dot}
      {label}
      {remove}
    </>
  );

  if (onClick) {
    return (
      <button
        type="button"
        aria-label={ariaLabel ?? `Tag ${name}`}
        onClick={onClick}
        data-slot="tag-chip"
        data-variant={variant}
        className={baseClasses}
        style={style}
      >
        {children}
      </button>
    );
  }

  return (
    <span
      data-slot="tag-chip"
      data-variant={variant}
      title={ariaLabel}
      className={baseClasses}
      style={style}
    >
      {children}
    </span>
  );
}
