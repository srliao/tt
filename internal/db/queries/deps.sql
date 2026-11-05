-- name: AddTaskDep :exec
INSERT OR IGNORE INTO task_deps (task_id, depends_on_id) VALUES (?, ?);

-- name: DeleteTaskDep :exec
DELETE FROM task_deps WHERE task_id = ? AND depends_on_id = ?;

-- name: TaskDepExists :one
SELECT EXISTS (
  SELECT 1 FROM task_deps WHERE task_id = ? AND depends_on_id = ?
);

-- name: ListTaskDeps :many
SELECT task_id, depends_on_id FROM task_deps
ORDER BY task_id, depends_on_id;

-- Both slices carry the same task ids: every edge with either end in the set.

-- name: ListTaskDepsTouching :many
SELECT d.task_id, d.depends_on_id, b.state AS depends_on_state
FROM task_deps d JOIN tasks b ON b.id = d.depends_on_id
WHERE d.task_id IN (sqlc.slice('task_ids'))
   OR d.depends_on_id IN (sqlc.slice('depends_on_ids'))
ORDER BY d.task_id, d.depends_on_id;

-- A cancelled blocker still blocks until the edge is removed.

-- name: ListUnfinishedBlockers :many
SELECT d.depends_on_id FROM task_deps d JOIN tasks b ON b.id = d.depends_on_id
WHERE d.task_id = ? AND b.state != 'done'
ORDER BY d.depends_on_id;

-- name: ListOpenDependents :many
SELECT d.task_id FROM task_deps d JOIN tasks t ON t.id = d.task_id
WHERE d.depends_on_id = ? AND t.state = 'not_done'
ORDER BY d.task_id;
