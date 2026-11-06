-- name: CreateTask :one
INSERT INTO tasks (title, notes, due_date, project, priority, staged_order, spawned_by_script_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = ?;

-- name: GetTaskTags :many
SELECT t.id, t.name
FROM tags t JOIN task_tags tt ON tt.tag_id = t.id
WHERE tt.task_id = ?
ORDER BY t.name;

-- name: UpdateTaskFields :one
UPDATE tasks
SET title = ?, notes = ?, due_date = ?, project = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: SetTaskState :one
UPDATE tasks
SET state = ?, completed_at = ?, cancelled_at = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: SetTaskStaged :one
UPDATE tasks SET staged_order = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: SetTaskPriority :one
UPDATE tasks SET priority = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = ?;

-- name: ClearStage :exec
UPDATE tasks SET staged_order = NULL, updated_at = ?
WHERE staged_order IS NOT NULL;

-- name: ClearFinishedFromStage :exec
UPDATE tasks SET staged_order = NULL, updated_at = ?
WHERE staged_order IS NOT NULL AND state IN ('done','cancelled');

-- New rows are minted at MIN(key) - 1 so they sort above everything already
-- present on the ascending-key axis. The 1.0 fallback makes the first row in
-- an empty list land on 0.0.

-- name: MinPriority :one
SELECT COALESCE(MIN(priority), 1.0) FROM tasks;

-- name: MinStagedOrder :one
SELECT COALESCE(MIN(staged_order), 1.0) FROM tasks WHERE staged_order IS NOT NULL;

-- name: ListAllPrioritiesAsc :many
SELECT id, priority FROM tasks ORDER BY priority ASC, id ASC;

-- name: ListAllStagedAsc :many
SELECT id, staged_order FROM tasks WHERE staged_order IS NOT NULL ORDER BY staged_order ASC, id ASC;

-- name: ListTasksByScript :many
SELECT * FROM tasks WHERE spawned_by_script_id = ?
ORDER BY created_at DESC, id DESC;

-- Ordered by the position in spawned_task_ids (j.key), NOT by rowid: a
-- spawning run inserts its batch in reverse so the tasks stack top-down in
-- spawn order, which leaves rowids running opposite to logical batch order.

-- name: ListLatestSpawnedTasksByScript :many
SELECT t.* FROM tasks t
JOIN json_each((
    SELECT spawned_task_ids FROM script_runs
    WHERE script_id = ? AND status = 'ok' AND spawned_task_ids != '[]'
    ORDER BY started_at DESC, id DESC LIMIT 1
)) j ON t.id = CAST(j.value AS INTEGER)
ORDER BY j.key ASC;

-- name: AddTaskTag :exec
INSERT OR IGNORE INTO task_tags (task_id, tag_id) VALUES (?, ?);

-- name: ReplaceTaskTags :exec
DELETE FROM task_tags WHERE task_id = ?;

-- name: DeleteTaskTagsForTask :exec
DELETE FROM task_tags WHERE task_id = ? AND tag_id IN (sqlc.slice('tag_ids'));

-- name: SetTaskProject :one
UPDATE tasks SET project = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: ListProjects :many
SELECT CAST(project AS TEXT) AS name,
       CAST(SUM(state = 'not_done') AS INTEGER) AS open_count,
       COUNT(*) AS total_count
FROM tasks
WHERE project IS NOT NULL
GROUP BY project
ORDER BY project;

-- name: RenameProject :execrows
UPDATE tasks SET project = sqlc.arg('to'), updated_at = sqlc.arg('updated_at')
WHERE project = sqlc.arg('from');

-- name: GetProject :one
SELECT CAST(project AS TEXT) AS name,
       CAST(SUM(state = 'not_done') AS INTEGER) AS open_count,
       COUNT(*) AS total_count
FROM tasks
WHERE project = ?
GROUP BY project;
