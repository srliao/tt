/**
 * <BulkTagEditor> — popover-based multi-task tag operations.
 *
 * Phase 6 of the keyboard + multi-select work. Anchored to the bulk action
 * bar's "Tag…" button, this editor exposes three modes — Add / Remove / Set
 * — and uses tri-state checkboxes to show which tags currently exist on
 * **all**, **some**, or **none** of the selected tasks. Tag changes are
 * staged client-side (chips at the top) and only flushed when the user
 * commits (Apply button or ⌘↵). Esc discards the staged set without
 * mutating anything.
 *
 * Why a fresh component instead of reusing `<TagCombobox>` directly:
 *   - Tri-state indicators have no analog in the single-task combobox.
 *   - Mode tabs + the "Set replaces all tags" confirm pattern would clutter
 *     the existing API for every other caller.
 *   - The flow is "stage many ops, commit once" — a different mental model
 *     from the single-task editor which patches on each toggle.
 *
 * We do reuse the cmdk vocabulary (filtering, "Create tag" row, paste with
 * commas) by hand so the keyboard ergonomics match the rest of the app.
 */

import { Command as CommandPrimitive } from 'cmdk';
import { AlertTriangleIcon, PlusIcon } from 'lucide-react';
import * as React from 'react';

import { useTagsWithCounts } from '@/api/tags';
import { useBulkTag } from '@/api/tasks';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { TagChip } from '@/components/ui/tag-chip';
import { cn } from '@/lib/utils';
import type { Task } from '@/types/task';
import { TriStateIndicator } from './bulk-tag-editor.tristate';

type Mode = 'add' | 'remove' | 'set';

const MODE_LABELS: Record<Mode, string> = {
  add: 'Add',
  remove: 'Remove',
  set: 'Set',
};

const MODE_DESCRIPTIONS: Record<Mode, string> = {
  add: 'Existing tags stay. New ones are added on top.',
  remove: 'Selected tags are stripped from every task. Tasks without them are unaffected.',
  set: 'Replaces all tags. Tasks end up with exactly the chosen set.',
};

const CREATE_VALUE = '__create__';

export interface BulkTagEditorProps {
  /** Tasks currently selected (full objects so we can compute tri-state). */
  selectedTasks: Task[];
  open: boolean;
  onOpenChange: (next: boolean) => void;
  /** Anchor element for the popover — usually the "Tag…" button in the bulk bar. */
  anchorRef: React.RefObject<HTMLElement | null>;
  /** Called after a successful apply. Page closes the editor via `onOpenChange`. */
  onApplied?: () => void;
}

function splitPaste(text: string): string[] {
  return text
    .split(/[,\n\r\t ]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export function BulkTagEditor({
  selectedTasks,
  open,
  onOpenChange,
  anchorRef,
  onApplied,
}: BulkTagEditorProps) {
  const [mode, setMode] = React.useState<Mode>('add');
  const [query, setQuery] = React.useState('');
  const [staged, setStaged] = React.useState<string[]>([]);
  const [confirmSet, setConfirmSet] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const inputRef = React.useRef<HTMLInputElement>(null);

  const bulkTag = useBulkTag();
  const tagsQuery = useTagsWithCounts();
  const allTags = React.useMemo(
    () => (tagsQuery.data ?? []).map((t) => ({ name: t.name, count: t.count })),
    [tagsQuery.data],
  );

  // Reset transient state when the editor is closed so reopening starts fresh.
  React.useEffect(() => {
    if (!open) {
      setMode('add');
      setQuery('');
      setStaged([]);
      setConfirmSet(false);
      setError(null);
    }
  }, [open]);

  // Tri-state computation — cached against the selection. The map carries the
  // raw count; rendering converts that to 'all'/'some'/'none' on the fly so we
  // don't have to recompute when callers ask for a count alongside state.
  const total = selectedTasks.length;
  const presenceMap = React.useMemo(() => {
    const m = new Map<string, number>();
    for (const t of selectedTasks) {
      for (const tag of t.tags) {
        m.set(tag, (m.get(tag) ?? 0) + 1);
      }
    }
    return m;
  }, [selectedTasks]);

  const stateFor = React.useCallback(
    (tag: string): 'all' | 'some' | 'none' => {
      const n = presenceMap.get(tag) ?? 0;
      if (total > 0 && n === total) return 'all';
      if (n > 0) return 'some';
      return 'none';
    },
    [presenceMap, total],
  );

  const countFor = React.useCallback((tag: string) => presenceMap.get(tag) ?? 0, [presenceMap]);

  // Union of "all known tags" + "tags actually present on the selection",
  // so a tag that exists only on selected tasks (perhaps not yet in the
  // global list because the cache is stale) still appears.
  const candidateTags = React.useMemo(() => {
    const seen = new Set<string>();
    const out: { name: string; count?: number }[] = [];
    for (const t of allTags) {
      seen.add(t.name);
      out.push(t);
    }
    for (const name of presenceMap.keys()) {
      if (!seen.has(name)) {
        seen.add(name);
        out.push({ name });
      }
    }
    return out;
  }, [allTags, presenceMap]);

  const trimmedQuery = query.trim();
  const queryLc = trimmedQuery.toLowerCase();
  const filtered = React.useMemo(() => {
    if (!queryLc) return candidateTags;
    return candidateTags.filter((t) => t.name.toLowerCase().includes(queryLc));
  }, [candidateTags, queryLc]);

  const hasExactMatch = React.useMemo(
    () => candidateTags.some((t) => t.name.toLowerCase() === queryLc),
    [candidateTags, queryLc],
  );
  const stagedSet = React.useMemo(() => new Set(staged), [staged]);
  const showCreate =
    mode !== 'remove' && trimmedQuery.length > 0 && !hasExactMatch && !stagedSet.has(trimmedQuery);

  const toggleStaged = React.useCallback((name: string) => {
    setStaged((prev) => (prev.includes(name) ? prev.filter((v) => v !== name) : [...prev, name]));
  }, []);

  const stageMany = React.useCallback((names: string[]) => {
    setStaged((prev) => {
      const next = prev.slice();
      const seen = new Set(prev);
      for (const n of names) {
        if (!seen.has(n)) {
          next.push(n);
          seen.add(n);
        }
      }
      return next;
    });
  }, []);

  // Whether clicking a row in the current mode is meaningful. Skipping the
  // no-op cases keeps the staged set tidy and prevents misleading chips.
  const isNoOp = React.useCallback(
    (tag: string): boolean => {
      if (mode === 'add') return stateFor(tag) === 'all';
      if (mode === 'remove') return stateFor(tag) === 'none';
      return false; // set mode always toggles target inclusion
    },
    [mode, stateFor],
  );

  const onSelectTag = React.useCallback(
    (name: string) => {
      if (isNoOp(name)) return;
      toggleStaged(name);
    },
    [isNoOp, toggleStaged],
  );

  const create = React.useCallback(() => {
    const name = trimmedQuery;
    if (!name) return;
    if (!stagedSet.has(name)) {
      setStaged((prev) => [...prev, name]);
    }
    setQuery('');
  }, [stagedSet, trimmedQuery]);

  const removeStaged = React.useCallback((name: string) => {
    setStaged((prev) => prev.filter((v) => v !== name));
  }, []);

  // ⌘↵ commits. In Set mode the confirm checkbox gates the apply. Empty
  // staged + non-Set is a no-op so users hammering ⌘↵ with nothing queued
  // don't accidentally fire a mutation.
  const canApply =
    !bulkTag.isPending &&
    selectedTasks.length > 0 &&
    (mode === 'set' ? confirmSet : staged.length > 0);

  const onApply = React.useCallback(async () => {
    if (!canApply) return;
    try {
      await bulkTag.mutateAsync({
        ids: selectedTasks.map((t) => t.id),
        op: mode,
        tags: staged,
      });
      setStaged([]);
      setQuery('');
      setConfirmSet(false);
      setError(null);
      onApplied?.();
      onOpenChange(false);
    } catch (err) {
      console.error('Bulk tag apply failed:', err);
      setError(err instanceof Error ? err.message : 'Failed to apply tag changes');
    }
  }, [bulkTag, canApply, mode, onApplied, onOpenChange, selectedTasks, staged]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // ⌘↵ / Ctrl+↵ commits from anywhere inside the popover.
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void onApply();
    }
  };

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Backspace' && query === '' && staged.length > 0) {
      e.preventDefault();
      setStaged((prev) => prev.slice(0, -1));
      return;
    }
    if (e.key === 'Escape') {
      // Defer to Radix's outer dismissal — the popover root closes via
      // `onOpenChange(false)` and our cleanup effect handles state reset.
      e.preventDefault();
      onOpenChange(false);
      return;
    }
  };

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text');
    if (!text) return;
    const parts = splitPaste(text);
    if (parts.length <= 1) return;
    e.preventDefault();
    stageMany(parts);
    setQuery('');
  };

  if (!open || selectedTasks.length === 0) return null;

  const anchorEl = anchorRef.current;
  if (!anchorEl) return null;

  return (
    <Popover
      open
      onOpenChange={(next) => {
        if (bulkTag.isPending) return;
        onOpenChange(next);
      }}
    >
      <PopoverAnchor virtualRef={{ current: anchorEl }} />
      <PopoverContent
        align="center"
        side="top"
        sideOffset={8}
        className="w-[22rem] gap-0 p-0"
        data-slot="bulk-tag-editor"
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          inputRef.current?.focus();
        }}
        onKeyDown={handleKeyDown}
      >
        <CommandPrimitive shouldFilter={false} loop={false}>
          {/* Header */}
          <div className="border-b px-3 py-2">
            <div className="flex items-baseline justify-between">
              <h2 className="text-sm font-medium">
                Bulk tag · {selectedTasks.length} task{selectedTasks.length === 1 ? '' : 's'}
              </h2>
            </div>
            <p
              className="mt-0.5 text-xs text-muted-foreground"
              data-slot="bulk-tag-editor-mode-description"
            >
              {MODE_DESCRIPTIONS[mode]}
            </p>
          </div>

          {/* Mode tabs */}
          <div className="flex gap-1 border-b px-2 py-2" role="tablist" aria-label="Tag operation">
            {(Object.keys(MODE_LABELS) as Mode[]).map((m) => {
              const isActive = mode === m;
              const isDestructive = m === 'set';
              return (
                <button
                  key={m}
                  type="button"
                  role="tab"
                  aria-selected={isActive}
                  data-slot="bulk-tag-editor-mode"
                  data-mode={m}
                  data-active={isActive || undefined}
                  data-destructive={isDestructive || undefined}
                  onClick={() => {
                    setMode(m);
                    setStaged([]);
                    setConfirmSet(false);
                  }}
                  className={cn(
                    'flex-1 rounded-md px-2 py-1 text-xs font-medium transition-colors',
                    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
                    isActive
                      ? isDestructive
                        ? 'bg-destructive text-destructive-foreground ring-1 ring-destructive/40'
                        : 'bg-foreground text-background'
                      : isDestructive
                        ? 'text-destructive/80 hover:bg-destructive/10'
                        : 'text-muted-foreground hover:bg-muted',
                  )}
                >
                  {MODE_LABELS[m]}
                </button>
              );
            })}
          </div>

          {/* Staged chips */}
          {staged.length > 0 && (
            <div
              className="flex flex-wrap items-center gap-1.5 border-b px-3 py-2"
              data-slot="bulk-tag-editor-staged"
            >
              {staged.map((name) => (
                <span key={name} className="inline-flex items-center gap-0.5">
                  <span aria-hidden className="text-[10px] font-mono text-muted-foreground">
                    {mode === 'remove' ? '−' : '+'}
                  </span>
                  <TagChip name={name} size="sm" onRemove={() => removeStaged(name)} />
                </span>
              ))}
              <span className="ml-auto text-[11px] text-muted-foreground">
                {staged.length} staged · <kbd className="font-mono">⌘↵</kbd>
              </span>
            </div>
          )}

          {/* Input */}
          <div className="border-b px-2 py-1.5">
            <CommandPrimitive.Input
              ref={inputRef}
              value={query}
              onValueChange={setQuery}
              onKeyDown={handleInputKeyDown}
              onPaste={handlePaste}
              placeholder={
                mode === 'remove' ? 'Search tags to remove…' : 'Type to filter or create…'
              }
              data-slot="bulk-tag-editor-input"
              autoComplete="off"
              spellCheck={false}
              className="w-full bg-transparent px-1 py-1 text-sm outline-none placeholder:text-muted-foreground"
            />
          </div>

          {/* Tag list */}
          <CommandPrimitive.List
            className="max-h-64 overflow-y-auto p-1"
            data-slot="bulk-tag-editor-list"
          >
            {filtered.length === 0 && !showCreate && (
              <CommandPrimitive.Empty className="py-4 text-center text-sm text-muted-foreground">
                No tags found.
              </CommandPrimitive.Empty>
            )}
            {filtered.length > 0 && (
              <CommandPrimitive.Group>
                {filtered.map((tag) => {
                  const presenceState = stateFor(tag.name);
                  const isStaged = stagedSet.has(tag.name);
                  const noOp = isNoOp(tag.name);
                  const count = countFor(tag.name);
                  return (
                    <CommandPrimitive.Item
                      key={tag.name}
                      value={`tag:${tag.name.toLowerCase()}`}
                      disabled={noOp}
                      onSelect={() => onSelectTag(tag.name)}
                      data-slot="bulk-tag-editor-item"
                      data-tag-name={tag.name}
                      data-presence={presenceState}
                      data-staged={isStaged || undefined}
                      data-noop={noOp || undefined}
                      className={cn(
                        'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm select-none',
                        'data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground',
                        noOp && 'cursor-default opacity-50',
                      )}
                    >
                      <TriStateIndicator mode={mode} presence={presenceState} staged={isStaged} />
                      <TagChip name={tag.name} size="sm" />
                      <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                        {presenceCountLabel(presenceState, count, total)}
                      </span>
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
                <CommandPrimitive.Group heading="Create">
                  <CommandPrimitive.Item
                    value={CREATE_VALUE}
                    onSelect={create}
                    data-slot="bulk-tag-editor-create"
                    className={cn(
                      'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm select-none',
                      'data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground',
                    )}
                  >
                    <PlusIcon className="size-4" />
                    <span>
                      Create tag <span className="font-semibold">“{trimmedQuery}”</span>
                    </span>
                  </CommandPrimitive.Item>
                </CommandPrimitive.Group>
              </>
            )}
          </CommandPrimitive.List>

          {/* Set-mode confirm + footer */}
          <div className="flex flex-col gap-2 border-t px-3 py-2">
            {mode === 'set' && (
              <label
                className="flex cursor-pointer items-start gap-2 text-xs text-destructive"
                data-slot="bulk-tag-editor-set-confirm"
              >
                <input
                  type="checkbox"
                  checked={confirmSet}
                  onChange={(e) => setConfirmSet(e.target.checked)}
                  className="mt-0.5"
                />
                <AlertTriangleIcon
                  className="mt-0.5 size-3.5 shrink-0"
                  aria-hidden
                  data-slot="bulk-tag-editor-set-warning"
                />
                <span>
                  I understand this replaces all tags
                  {staged.length === 0 && ' (clears every tag)'}.
                </span>
              </label>
            )}
            {error && (
              <p
                className="text-xs text-destructive"
                role="alert"
                data-slot="bulk-tag-editor-error"
              >
                {error}
              </p>
            )}
            <div className="flex items-center justify-between gap-2">
              <span className="text-[11px] text-muted-foreground">
                <kbd className="font-mono">↑↓</kbd> nav · <kbd className="font-mono">↵</kbd> stage ·{' '}
                <kbd className="font-mono">Esc</kbd> cancel
              </span>
              <button
                type="button"
                onClick={() => void onApply()}
                disabled={!canApply}
                data-slot="bulk-tag-editor-apply"
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-md bg-foreground px-2.5 py-1 text-xs font-medium text-background shadow-sm transition-colors hover:bg-foreground/90',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
                )}
              >
                {bulkTag.isPending ? 'Applying…' : 'Apply'}
                <kbd className="font-mono text-[10px] opacity-80">⌘↵</kbd>
              </button>
            </div>
          </div>
        </CommandPrimitive>
      </PopoverContent>
    </Popover>
  );
}

function presenceCountLabel(
  presence: 'all' | 'some' | 'none',
  count: number,
  total: number,
): string {
  if (presence === 'all') return `all ${total}`;
  if (presence === 'none') return `0 of ${total}`;
  return `${count} of ${total}`;
}
