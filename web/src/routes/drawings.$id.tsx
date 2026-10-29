import { createFileRoute } from '@tanstack/react-router';
import { DrawingEditorPage } from '@/features/drawings/editor-page';

function DrawingDetailPage() {
  const { id } = Route.useParams();
  const numericId = Number.parseInt(id, 10);
  if (!Number.isFinite(numericId)) {
    return <div className="p-6">Invalid drawing id.</div>;
  }
  // Keyed so navigating between drawings remounts the canvas with fresh data.
  return <DrawingEditorPage key={numericId} id={numericId} />;
}

export const Route = createFileRoute('/drawings/$id')({
  component: DrawingDetailPage,
});
