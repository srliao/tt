-- name: CreateTag :one
INSERT INTO tags (name) VALUES (?) RETURNING *;

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = ?;

-- name: GetTagByID :one
SELECT * FROM tags WHERE id = ?;

-- name: ListTags :many
SELECT * FROM tags ORDER BY name ASC;

-- name: RenameTag :one
UPDATE tags SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ?;
