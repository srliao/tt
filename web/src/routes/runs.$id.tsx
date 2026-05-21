import { createFileRoute } from '@tanstack/react-router';

function RunDetailPage() {
  const { id } = Route.useParams();
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">Run #{id} (todo: phase 08e)</h1>
    </div>
  );
}

export const Route = createFileRoute('/runs/$id')({
  component: RunDetailPage,
});
