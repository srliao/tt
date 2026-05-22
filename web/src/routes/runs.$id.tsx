import { createFileRoute } from '@tanstack/react-router';
import { RunDetailPage } from '@/features/runs/detail-page';

function RouteComponent() {
  const { id } = Route.useParams();
  const numericId = Number(id);
  if (!Number.isFinite(numericId) || numericId <= 0) {
    return (
      <section className="mx-auto w-full max-w-5xl px-4 py-4">
        <p className="text-sm text-destructive">Invalid run id: {id}</p>
      </section>
    );
  }
  return <RunDetailPage id={numericId} />;
}

export const Route = createFileRoute('/runs/$id')({
  component: RouteComponent,
});
