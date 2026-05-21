import { createFileRoute } from '@tanstack/react-router';

function ScriptsIndexPage() {
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">Scripts list (todo: phase 08d)</h1>
    </div>
  );
}

export const Route = createFileRoute('/scripts/')({
  component: ScriptsIndexPage,
});
