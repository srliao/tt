import { createFileRoute } from '@tanstack/react-router';

function StagePage() {
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">Stage page (todo: phase 08b)</h1>
    </div>
  );
}

export const Route = createFileRoute('/stage')({
  component: StagePage,
});
