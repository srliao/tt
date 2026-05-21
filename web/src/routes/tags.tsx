import { createFileRoute } from '@tanstack/react-router';
import { TagsPage } from '@/features/tags/page';

export const Route = createFileRoute('/tags')({
  component: TagsPage,
});
