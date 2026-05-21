/**
 * Form rendered at the top of the /tags page for creating a new tag.
 *
 * Validation: trimmed name must be non-empty. The server enforces uniqueness
 * with a 409 `conflict` envelope — we catch the `ApiError` and surface the
 * message inline rather than letting it bubble up as an unhandled rejection.
 */

import { zodResolver } from '@hookform/resolvers/zod';
import { PlusIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { useCreateTag } from '@/api/tags';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ApiError } from '@/lib/api';

const schema = z.object({
  name: z.string().trim().min(1, 'Tag name is required'),
});

type FormValues = z.infer<typeof schema>;

export function AddTagForm() {
  const createTag = useCreateTag();
  const [conflictError, setConflictError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '' },
  });

  const onSubmit = handleSubmit(async (values) => {
    setConflictError(null);
    try {
      await createTag.mutateAsync(values.name.trim());
      reset({ name: '' });
    } catch (err) {
      if (err instanceof ApiError && err.code === 'conflict') {
        setConflictError('Tag already exists');
        return;
      }
      throw err;
    }
  });

  return (
    <form
      onSubmit={onSubmit}
      onChange={() => {
        if (conflictError) setConflictError(null);
      }}
      className="flex flex-col gap-1.5"
      aria-label="Add tag"
    >
      <Label htmlFor="add-tag-name" className="sr-only">
        Tag name
      </Label>
      <div className="flex items-start gap-2">
        <div className="flex flex-1 flex-col gap-1">
          <Input
            id="add-tag-name"
            placeholder="New tag name"
            autoComplete="off"
            aria-invalid={!!errors.name || !!conflictError}
            {...register('name')}
          />
          {errors.name && (
            <p className="text-xs text-destructive" role="alert">
              {errors.name.message}
            </p>
          )}
          {conflictError && (
            <p className="text-xs text-destructive" role="alert">
              {conflictError}
            </p>
          )}
        </div>
        <Button type="submit" disabled={isSubmitting}>
          <PlusIcon /> Add tag
        </Button>
      </div>
    </form>
  );
}
