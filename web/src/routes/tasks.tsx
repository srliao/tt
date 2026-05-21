import { createFileRoute } from '@tanstack/react-router';

function TasksPage() {
  return (
    <div className="p-6">
      <h1 className="text-2xl font-semibold">Tasks page (todo: phase 08a)</h1>
    </div>
  );
}

export const Route = createFileRoute('/tasks')({
  component: TasksPage,
});
