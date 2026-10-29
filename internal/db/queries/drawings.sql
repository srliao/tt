-- name: CreateDrawing :one
INSERT INTO drawings (name, scene) VALUES (?, ?) RETURNING *;

-- name: GetDrawing :one
SELECT * FROM drawings WHERE id = ?;

-- name: ListDrawings :many
SELECT id, name, created_at, updated_at FROM drawings ORDER BY updated_at DESC, id DESC;

-- name: UpdateDrawing :one
-- NULL leaves a column unchanged, so a PATCH of name and/or scene is one
-- atomic statement.
UPDATE drawings
SET name = COALESCE(sqlc.narg('name'), name),
    scene = COALESCE(sqlc.narg('scene'), scene),
    updated_at = datetime('now')
WHERE id = sqlc.arg('id') RETURNING *;

-- name: DeleteDrawing :exec
DELETE FROM drawings WHERE id = ?;
