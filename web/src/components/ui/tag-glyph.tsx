/**
 * <TagGlyph> + <TagGlyphList> — compact, row-only tag representation.
 *
 * Renders a tag as a 16x16 colored square with a 1- or 2-letter initial
 * (computed by `buildInitialMap` in `@/lib/tag-initials`) plus a Radix
 * tooltip carrying the full tag name. Click semantics match the full
 * `<TagChip>`: replace by default, shift-click to add, alt-click to exclude
 * — caller wires that via `onTagClick`.
 *
 * Full tag chips (sidebar / modal / palette / active-filter strip) keep
 * using `<TagChip>` from `tag-chip.tsx`. Only the task row uses the glyph
 * presentation.
 */

import type * as React from 'react';

import { useTagHueMap } from '@/api/tags';
import { useTheme } from '@/components/theme-provider';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { tagColor, tagColorDark } from '@/lib/tag-color';
import { cn } from '@/lib/utils';

export interface TagGlyphProps {
  name: string;
  initial: string;
  onClick?: (event: React.MouseEvent) => void;
}

export function TagGlyph({ name, initial, onClick }: TagGlyphProps) {
  const { resolvedTheme } = useTheme();
  const hueMap = useTagHueMap();
  const hue = hueMap.get(name);
  const palette = resolvedTheme === 'dark' ? tagColorDark(name, hue) : tagColor(name, hue);

  return (
    <Tooltip delayDuration={120}>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onClick}
          aria-label={`Tag ${name}`}
          data-tag={name}
          data-slot="tag-glyph"
          className={cn(
            'inline-flex size-4 items-center justify-center rounded-[4px]',
            'font-mono text-[9px] font-semibold leading-none tracking-tight',
            'transition-[transform,box-shadow]',
            'hover:scale-110 hover:shadow-sm',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary',
          )}
          style={{ backgroundColor: palette.bg, color: palette.fg }}
        >
          {initial}
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" sideOffset={4}>
        <span className="font-mono text-xs">{name}</span>
      </TooltipContent>
    </Tooltip>
  );
}

export interface TagGlyphListProps {
  tags: string[];
  initialMap: Map<string, string>;
  onTagClick: (name: string, event: React.MouseEvent) => void;
}

/** Cap visible glyphs to keep the Tags column from overflowing. */
export const TAG_GLYPH_MAX = 5;

export function TagGlyphList({ tags, initialMap, onTagClick }: TagGlyphListProps) {
  const visible = tags.slice(0, TAG_GLYPH_MAX);
  const overflow = tags.length - TAG_GLYPH_MAX;
  const hidden = overflow > 0 ? tags.slice(TAG_GLYPH_MAX) : [];

  return (
    <TooltipProvider>
      <div className="inline-flex items-center gap-[3px]" data-tag-cell>
        {visible.map((name) => (
          <TagGlyph
            key={name}
            name={name}
            initial={initialMap.get(name) ?? (name[0] ?? '?').toUpperCase()}
            onClick={(e) => onTagClick(name, e)}
          />
        ))}
        {overflow > 0 && (
          <Tooltip delayDuration={120}>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={`${overflow} more tag${overflow === 1 ? '' : 's'}`}
                data-slot="tag-glyph-overflow"
                className={cn(
                  'inline-flex h-4 min-w-4 items-center justify-center rounded-[4px] px-1',
                  'bg-muted text-muted-foreground',
                  'font-mono text-[9px] font-semibold leading-none tracking-tight',
                  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary',
                )}
              >
                +{overflow}
              </button>
            </TooltipTrigger>
            <TooltipContent side="top" sideOffset={4}>
              <span className="font-mono text-xs">{hidden.join(' · ')}</span>
            </TooltipContent>
          </Tooltip>
        )}
      </div>
    </TooltipProvider>
  );
}
