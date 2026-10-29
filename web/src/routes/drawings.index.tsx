import { createFileRoute } from '@tanstack/react-router';
import { DrawingsListPage } from '@/features/drawings/list-page';

export const Route = createFileRoute('/drawings/')({
  component: DrawingsListPage,
});
