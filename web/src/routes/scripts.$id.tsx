import { createFileRoute } from '@tanstack/react-router';

function ScriptDetailPage() {
  const { id } = Route.useParams();
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">
        Script #{id} (todo: phase 08d)
      </h1>
    </div>
  );
}

export const Route = createFileRoute('/scripts/$id')({
  component: ScriptDetailPage,
});
