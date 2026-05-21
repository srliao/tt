-- name: CreateScript :one
INSERT INTO scripts (name, code, enabled, schedule_kind, schedule_config)
VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: GetScript :one
SELECT * FROM scripts WHERE id = ?;

-- name: ListScripts :many
SELECT * FROM scripts ORDER BY name ASC, id ASC;

-- name: ListEnabledScripts :many
SELECT * FROM scripts WHERE enabled = 1;

-- name: UpdateScript :one
UPDATE scripts
SET name = ?, code = ?, enabled = ?, schedule_kind = ?, schedule_config = ?,
    updated_at = datetime('now')
WHERE id = ? RETURNING *;

-- name: DeleteScript :exec
DELETE FROM scripts WHERE id = ?;

-- name: SetScriptLastRunAt :exec
UPDATE scripts SET last_run_at = ?, updated_at = datetime('now') WHERE id = ?;

-- name: SetScriptUserState :exec
UPDATE scripts SET user_state = ?, updated_at = datetime('now') WHERE id = ?;

-- name: GetScriptUserState :one
SELECT user_state FROM scripts WHERE id = ?;
