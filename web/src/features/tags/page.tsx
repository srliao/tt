/**
 * The /tags page. Lists all tags alphabetically with a creation form at the
 * top. Each `<TagRow>` owns its own rename + delete affordances.
 *
 * Per spec §6 we always render the AddTagForm; the list area shows an
 * informational placeholder when no tags exist yet rather than swapping out
 * the whole page.
 */

import { useMemo } from 'react';
import { useTags } from '@/api/tags';
import { AddTagForm } from './add-tag-form';
import { TagRow } from './tag-row';

export function TagsPage() {
  const { data: tags = [], isLoading } = useTags();

  const sorted = useMemo(() => [...tags].sort((a, b) => a.name.localeCompare(b.name)), [tags]);

  return (
    <section className="mx-auto flex w-full max-w-2xl flex-col gap-4 px-4 py-4">
      <header>
        <h1 className="font-heading text-xl font-medium">Tags</h1>
        <p className="text-sm text-muted-foreground">Manage the labels you can attach to tasks.</p>
      </header>

      <AddTagForm />

      {isLoading ? null : sorted.length === 0 ? (
        <p className="rounded-md border border-dashed py-8 text-center text-sm text-muted-foreground">
          No tags yet. Add one to start organizing tasks.
        </p>
      ) : (
        <ul className="flex flex-col gap-2" aria-label="Tags">
          {sorted.map((tag) => (
            <TagRow key={tag.id} tag={tag} />
          ))}
        </ul>
      )}
    </section>
  );
}
