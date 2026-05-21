import { createFileRoute } from '@tanstack/react-router';
import { TasksPage } from '@/features/tasks/page';
import { taskSearchSchema } from '@/features/tasks/use-task-list-search';

export const Route = createFileRoute('/tasks')({
  validateSearch: (search) => taskSearchSchema.parse(search),
  component: TasksPage,
});
