import { createFileRoute } from '@tanstack/react-router';

function TagsPage() {
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">Tags page (todo: phase 08c)</h1>
    </div>
  );
}

export const Route = createFileRoute('/tags')({
  component: TagsPage,
});
