-- +goose Up

-- task_id depends on depends_on_id: the blocker must finish first. Whether a
-- task is blocked is derived from its blockers' states, never stored.
CREATE TABLE task_deps (
  task_id       INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  depends_on_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  created_at    TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (task_id, depends_on_id),
  CHECK (task_id != depends_on_id)
);

CREATE INDEX task_deps_depends_on_idx ON task_deps(depends_on_id);

-- +goose Down

DROP TABLE task_deps;
