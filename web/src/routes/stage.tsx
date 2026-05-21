import { createFileRoute } from '@tanstack/react-router';
import { StagePage } from '@/features/stage/page';

export const Route = createFileRoute('/stage')({
  component: StagePage,
});
