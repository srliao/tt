-- +goose Up

ALTER TABLE tasks ADD COLUMN project TEXT;

CREATE INDEX tasks_project_idx ON tasks(project);

-- +goose Down

DROP INDEX tasks_project_idx;

ALTER TABLE tasks DROP COLUMN project;
