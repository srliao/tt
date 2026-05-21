import { createFileRoute } from '@tanstack/react-router';

function ScriptNewPage() {
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">New script (todo: phase 08d)</h1>
    </div>
  );
}

export const Route = createFileRoute('/scripts/new')({
  component: ScriptNewPage,
});
