import { createFileRoute } from '@tanstack/react-router';
import { RunsListPage, runsSearchSchema } from '@/features/runs/list-page';

export const Route = createFileRoute('/runs/')({
  validateSearch: (search) => runsSearchSchema.parse(search),
  component: RunsListPage,
});
