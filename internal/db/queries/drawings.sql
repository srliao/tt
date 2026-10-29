-- name: CreateDrawing :one
INSERT INTO drawings (name, scene) VALUES (?, ?) RETURNING *;

-- name: GetDrawing :one
SELECT * FROM drawings WHERE id = ?;

-- name: ListDrawings :many
SELECT id, name, created_at, updated_at FROM drawings ORDER BY updated_at DESC, id DESC;

-- name: RenameDrawing :one
UPDATE drawings SET name = ?, updated_at = datetime('now') WHERE id = ? RETURNING *;

-- name: SetDrawingScene :one
UPDATE drawings SET scene = ?, updated_at = datetime('now') WHERE id = ? RETURNING *;

-- name: DeleteDrawing :exec
DELETE FROM drawings WHERE id = ?;
