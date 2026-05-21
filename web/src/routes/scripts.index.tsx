import { createFileRoute } from '@tanstack/react-router';
import { ScriptsListPage } from '@/features/scripts/list-page';

export const Route = createFileRoute('/scripts/')({
  component: ScriptsListPage,
});
