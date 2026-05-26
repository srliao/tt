-- name: CreateTag :one
INSERT INTO tags (name, color_hue) VALUES (?, ?) RETURNING *;

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = ?;

-- name: GetTagByID :one
SELECT * FROM tags WHERE id = ?;

-- name: ListTags :many
SELECT * FROM tags ORDER BY name ASC;

-- name: ListTagsWithCounts :many
SELECT t.id, t.name, t.color_hue, t.created_at, COALESCE(c.cnt, 0) AS count
FROM tags t
LEFT JOIN (
  SELECT tag_id, COUNT(DISTINCT task_id) AS cnt
  FROM task_tags
  GROUP BY tag_id
) c ON c.tag_id = t.id
ORDER BY t.name ASC;

-- name: RenameTag :one
UPDATE tags SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ?;

-- name: CountTagsByHue :many
SELECT color_hue, COUNT(*) AS count
FROM tags
GROUP BY color_hue;
