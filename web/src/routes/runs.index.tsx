import { createFileRoute } from '@tanstack/react-router';

function RunsIndexPage() {
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">Runs list (todo: phase 08e)</h1>
    </div>
  );
}

export const Route = createFileRoute('/runs/')({
  component: RunsIndexPage,
});
