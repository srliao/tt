'use client';

/**
 * <TagCombobox> — the single chip-input used everywhere tags are entered.
 *
 * Phase 1 component of the tt-ui-improvements work. Combines a multi-select
 * dropdown (cmdk) with a chip trigger so users can:
 *   - filter the list by typing
 *   - keyboard-navigate (↑ / ↓ / ↵)
 *   - toggle existing tags (↵)
 *   - create a brand-new tag (↵ on "Create" row, only when `allowCreate`)
 *   - remove the last chip with ⌫ on empty query
 *   - paste a comma/space-separated list to bulk-add
 *
 * Implementation notes
 * --------------------
 * The trigger is a plain `<div role="combobox">` (NOT a `<button>`) so the
 * embedded chip × buttons aren't nested inside another button — that would
 * be invalid HTML and break accessibility. The visible text input *is* the
 * cmdk `<Command.Input>` — using it directly means ↑/↓/↵ are wired natively
 * by cmdk without bridging keyboard events between two inputs.
 *
 * The Popover is anchored to the trigger via `<PopoverAnchor>`. Clicks on
 * chip × buttons inside the trigger must NOT dismiss the dropdown, so
 * `onPointerDownOutside` is intercepted when the event target is inside the
 * trigger element.
 */

import { Command as CommandPrimitive } from 'cmdk';
import { CheckIcon, PlusIcon } from 'lucide-react';
import * as React from 'react';

import { useCreateTag, useTagsWithCounts } from '@/api/tags';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { TagChip } from '@/components/ui/tag-chip';
import { cn } from '@/lib/utils';

export interface TagComboboxProps {
  /** Current set of selected tag names. */
  value: string[];
  /** Fires whenever a tag is added or removed. */
  onChange: (next: string[]) => void;
  /**
   * When set, allow creating a tag that doesn't exist yet (server POST).
   * Default `true` — callers in filter-only contexts should pass `false`.
   */
  allowCreate?: boolean;
  /**
   * Optional override for the source list. Defaults to `useTagsWithCounts()`.
   * Useful for unit tests and stories where we don't want a real query.
   */
  available?: { name: string; count?: number }[];
  /** Placeholder text for the inline input. */
  placeholder?: string;
  /**
   * Render mode. `'input'` (default) puts chips inline with the input, like a
   * traditional chip-input. `'block'` puts chips above a stationary list and
   * is used by the filter sidebar.
   */
  layout?: 'input' | 'block';
  /** Disable autofocus when the popover opens (palette uses this). */
  autoFocus?: boolean;
  /** Optional id for the trigger. */
  id?: string;
  /** Extra classes for the trigger element. */
  className?: string;
}

const CREATE_VALUE = '__create__';

function splitPaste(text: string): string[] {
  return text
    .split(/[,\n\r\t ]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export function TagCombobox({
  value,
  onChange,
  allowCreate = true,
  available,
  placeholder = 'Add tag…',
  layout: _layout = 'input',
  autoFocus = true,
  id,
  className,
}: TagComboboxProps) {
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState('');
  const inputRef = React.useRef<HTMLInputElement>(null);
  const triggerRef = React.useRef<HTMLDivElement>(null);

  const fallback = useTagsWithCounts();
  const tags = React.useMemo(() => {
    if (available) return available;
    return (fallback.data ?? []).map((t) => ({ name: t.name, count: t.count }));
  }, [available, fallback.data]);

  const createTag = useCreateTag();

  const trimmedQuery = query.trim();
  const queryLc = trimmedQuery.toLowerCase();
  const selectedSet = React.useMemo(() => new Set(value), [value]);

  const filtered = React.useMemo(() => {
    if (!queryLc) return tags;
    return tags.filter((t) => t.name.toLowerCase().includes(queryLc));
  }, [tags, queryLc]);

  const hasExactMatch = React.useMemo(
    () => tags.some((t) => t.name.toLowerCase() === queryLc),
    [tags, queryLc],
  );
  const alreadySelected = selectedSet.has(trimmedQuery);
  const showCreate = allowCreate && trimmedQuery.length > 0 && !hasExactMatch && !alreadySelected;

  const toggle = React.useCallback(
    (name: string) => {
      if (selectedSet.has(name)) {
        onChange(value.filter((v) => v !== name));
      } else {
        onChange([...value, name]);
      }
    },
    [onChange, selectedSet, value],
  );

  const addMany = React.useCallback(
    (names: string[]) => {
      const next = [...value];
      const seen = new Set(value);
      for (const n of names) {
        if (!seen.has(n)) {
          next.push(n);
          seen.add(n);
        }
      }
      if (next.length !== value.length) onChange(next);
    },
    [onChange, value],
  );

  const create = React.useCallback(async () => {
    const name = trimmedQuery;
    if (!name) return;
    try {
      await createTag.mutateAsync(name);
    } catch (err) {
      const code = (err as { code?: string } | undefined)?.code;
      // Conflict means it already exists — fall through and add by name.
      if (code !== 'conflict') throw err;
    }
    if (!selectedSet.has(name)) onChange([...value, name]);
    setQuery('');
  }, [createTag, onChange, selectedSet, trimmedQuery, value]);

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Backspace' && query === '' && value.length > 0) {
      e.preventDefault();
      onChange(value.slice(0, -1));
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      setOpen(false);
      return;
    }
  };

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text');
    if (!text) return;
    const parts = splitPaste(text);
    if (parts.length <= 1) return;
    e.preventDefault();
    addMany(parts);
    setQuery('');
  };

  const handleTriggerClick = (e: React.MouseEvent<HTMLDivElement>) => {
    // Don't refocus when the user clicked a button inside (eg. chip ×).
    if ((e.target as HTMLElement).closest('button')) return;
    setOpen(true);
    requestAnimationFrame(() => inputRef.current?.focus());
  };

  const removeAt = (name: string) => onChange(value.filter((v) => v !== name));

  // Single cmdk Command tree wraps the visible input AND the dropdown list.
  // The Input lives inside the trigger; the List inside the Popover. cmdk is
  // happy as long as everything shares one <Command> root, even across the
  // Radix portal boundary.
  return (
    <CommandPrimitive
      shouldFilter={false}
      className="w-full"
      // Disable cmdk's default loop so ↑ at top doesn't wrap awkwardly.
      loop={false}
    >
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverAnchor asChild>
          {/* biome-ignore lint/a11y/noStaticElementInteractions: focus delegation to inner input; the click handler exists only to focus the input when the user clicks the surrounding chip area */}
          <div
            ref={triggerRef}
            id={id}
            data-slot="tag-combobox-trigger"
            data-expanded={open || undefined}
            onClick={handleTriggerClick}
            className={cn(
              'flex min-h-9 w-full flex-wrap items-center gap-1.5 rounded-md border border-input bg-transparent px-2 py-1 text-sm transition-colors',
              'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50',
              'cursor-text',
              className,
            )}
          >
            {value.map((name) => (
              <TagChip key={name} name={name} size="sm" onRemove={() => removeAt(name)} />
            ))}
            <CommandPrimitive.Input
              ref={inputRef}
              value={query}
              onValueChange={(v) => {
                setQuery(v);
                if (!open) setOpen(true);
              }}
              placeholder={value.length === 0 ? placeholder : ''}
              onFocus={() => setOpen(true)}
              onKeyDown={handleInputKeyDown}
              onPaste={handlePaste}
              data-slot="tag-combobox-input"
              autoComplete="off"
              spellCheck={false}
              className="min-w-15 flex-1 bg-transparent text-sm placeholder:text-muted-foreground outline-none"
            />
          </div>
        </PopoverAnchor>
        <PopoverContent
          align="start"
          sideOffset={4}
          className="w-(--radix-popover-trigger-width) p-0"
          onOpenAutoFocus={(e) => {
            // Keep focus on the trigger input — never steal it into the
            // popover content (cmdk would otherwise focus its own root).
            e.preventDefault();
            if (autoFocus) inputRef.current?.focus();
          }}
          onCloseAutoFocus={(e) => {
            // Don't yank focus back to wherever it was before; the user is
            // still likely interacting with the input.
            e.preventDefault();
          }}
          onPointerDownOutside={(e) => {
            const target = e.target as Node | null;
            if (target && triggerRef.current?.contains(target)) {
              e.preventDefault();
            }
          }}
          onInteractOutside={(e) => {
            const target = e.target as Node | null;
            if (target && triggerRef.current?.contains(target)) {
              e.preventDefault();
            }
          }}
        >
          <CommandPrimitive.List className="max-h-64 overflow-y-auto p-1">
            {filtered.length === 0 && !showCreate && (
              <CommandPrimitive.Empty className="py-4 text-center text-sm text-muted-foreground">
                No tags found.
              </CommandPrimitive.Empty>
            )}
            {filtered.length > 0 && (
              <CommandPrimitive.Group>
                {filtered.map((tag) => {
                  const selected = selectedSet.has(tag.name);
                  const cmdValue = `tag:${tag.name.toLowerCase()}`;
                  return (
                    <CommandPrimitive.Item
                      key={tag.name}
                      value={cmdValue}
                      onSelect={() => toggle(tag.name)}
                      data-slot="tag-combobox-item"
                      data-tag-name={tag.name}
                      data-tag-selected={selected || undefined}
                      className={cn(
                        'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm select-none',
                        'data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground',
                      )}
                    >
                      <span
                        aria-hidden
                        className={cn(
                          'flex size-4 shrink-0 items-center justify-center rounded-lg border',
                          selected
                            ? 'border-primary bg-primary text-primary-foreground'
                            : 'border-input',
                        )}
                      >
                        {selected && <CheckIcon className="size-3" />}
                      </span>
                      <TagChip name={tag.name} size="sm" />
                      {tag.count != null && (
                        <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                          {tag.count}
                        </span>
                      )}
                    </CommandPrimitive.Item>
                  );
                })}
              </CommandPrimitive.Group>
            )}
            {showCreate && (
              <>
                {filtered.length > 0 && (
                  <CommandPrimitive.Separator className="-mx-1 my-1 h-px bg-border" />
                )}
                <CommandPrimitive.Group>
                  <CommandPrimitive.Item
                    value={CREATE_VALUE}
                    onSelect={() => {
                      void create();
                    }}
                    data-slot="tag-combobox-create"
                    className={cn(
                      'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm select-none',
                      'data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground',
                    )}
                  >
                    <PlusIcon className="size-4" />
                    <span>
                      Create <span className="font-semibold">“{trimmedQuery}”</span>
                    </span>
                  </CommandPrimitive.Item>
                </CommandPrimitive.Group>
              </>
            )}
          </CommandPrimitive.List>
        </PopoverContent>
      </Popover>
    </CommandPrimitive>
  );
}
