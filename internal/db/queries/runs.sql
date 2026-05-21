-- name: CreateScriptRun :one
INSERT INTO script_runs (script_id, trigger, status)
VALUES (?, ?, 'running') RETURNING *;

-- name: FinishScriptRun :exec
UPDATE script_runs
SET finished_at = datetime('now'), status = ?, error_message = ?,
    spawned_task_ids = ?
WHERE id = ?;

-- name: GetScriptRun :one
SELECT * FROM script_runs WHERE id = ?;

-- name: ListScriptRunsByScript :many
SELECT * FROM script_runs WHERE script_id = ?
ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?;

-- name: ListAllScriptRuns :many
SELECT * FROM script_runs ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?;

-- name: CountScriptRuns :one
SELECT COUNT(*) FROM script_runs;

-- name: DeleteOldestScriptRuns :exec
DELETE FROM script_runs WHERE id IN (
  SELECT id FROM script_runs ORDER BY started_at ASC, id ASC LIMIT ?
);

-- name: MarkOrphanedRunsAsError :exec
UPDATE script_runs
SET status = 'error', error_message = 'interrupted (binary restart)',
    finished_at = datetime('now')
WHERE status = 'running';

-- name: AppendScriptLog :exec
INSERT INTO script_logs (script_run_id, level, message) VALUES (?, ?, ?);

-- name: ListScriptLogsByRun :many
SELECT * FROM script_logs WHERE script_run_id = ?
ORDER BY logged_at ASC, id ASC;
