import { createFileRoute } from '@tanstack/react-router';
import { ScriptEditorPage } from '@/features/scripts/editor-page';

function ScriptDetailPage() {
  const { id } = Route.useParams();
  const numericId = Number.parseInt(id, 10);
  if (!Number.isFinite(numericId)) {
    return <div className="p-6">Invalid script id.</div>;
  }
  return <ScriptEditorPage id={numericId} />;
}

export const Route = createFileRoute('/scripts/$id')({
  component: ScriptDetailPage,
});
