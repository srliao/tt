import { createFileRoute } from '@tanstack/react-router';
import { ScriptEditorPage } from '@/features/scripts/editor-page';

export const Route = createFileRoute('/scripts/new')({
  component: () => <ScriptEditorPage />,
});
