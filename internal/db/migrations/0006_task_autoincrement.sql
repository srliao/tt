-- +goose NO TRANSACTION
-- +goose Up

-- AUTOINCREMENT keeps sqlite_sequence at the high-water mark, so the id of a
-- deleted task is never reissued. Copying rows with their explicit ids seeds
-- the sequence at max(id).
PRAGMA foreign_keys = OFF;

CREATE TABLE tasks_new (
  id                   INTEGER PRIMARY KEY AUTOINCREMENT,
  title                TEXT NOT NULL,
  notes                TEXT NOT NULL DEFAULT '',
  state                TEXT NOT NULL DEFAULT 'not_done'
                          CHECK (state IN ('not_done','done','cancelled')),
  due_date             TEXT,
  priority             REAL NOT NULL DEFAULT 0,
  staged_order         REAL,
  spawned_by_script_id INTEGER REFERENCES scripts(id) ON DELETE SET NULL,
  created_at           TEXT NOT NULL DEFAULT (datetime('now')),
  completed_at         TEXT,
  cancelled_at         TEXT,
  updated_at           TEXT NOT NULL DEFAULT (datetime('now')),
  project              TEXT
);

INSERT INTO tasks_new (
  id, title, notes, state, due_date, priority, staged_order, spawned_by_script_id,
  created_at, completed_at, cancelled_at, updated_at, project
)
SELECT
  id, title, notes, state, due_date, priority, staged_order, spawned_by_script_id,
  created_at, completed_at, cancelled_at, updated_at, project
FROM tasks;

DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;

CREATE INDEX tasks_state_idx         ON tasks(state);
CREATE INDEX tasks_due_date_idx      ON tasks(due_date);
CREATE INDEX tasks_priority_idx      ON tasks(priority);
CREATE INDEX tasks_staged_order_idx  ON tasks(staged_order) WHERE staged_order IS NOT NULL;
CREATE INDEX tasks_spawned_by_idx    ON tasks(spawned_by_script_id);
CREATE INDEX tasks_project_idx       ON tasks(project);

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;

-- +goose Down

PRAGMA foreign_keys = OFF;

CREATE TABLE tasks_new (
  id                   INTEGER PRIMARY KEY,
  title                TEXT NOT NULL,
  notes                TEXT NOT NULL DEFAULT '',
  state                TEXT NOT NULL DEFAULT 'not_done'
                          CHECK (state IN ('not_done','done','cancelled')),
  due_date             TEXT,
  priority             REAL NOT NULL DEFAULT 0,
  staged_order         REAL,
  spawned_by_script_id INTEGER REFERENCES scripts(id) ON DELETE SET NULL,
  created_at           TEXT NOT NULL DEFAULT (datetime('now')),
  completed_at         TEXT,
  cancelled_at         TEXT,
  updated_at           TEXT NOT NULL DEFAULT (datetime('now')),
  project              TEXT
);

INSERT INTO tasks_new (
  id, title, notes, state, due_date, priority, staged_order, spawned_by_script_id,
  created_at, completed_at, cancelled_at, updated_at, project
)
SELECT
  id, title, notes, state, due_date, priority, staged_order, spawned_by_script_id,
  created_at, completed_at, cancelled_at, updated_at, project
FROM tasks;

DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;

CREATE INDEX tasks_state_idx         ON tasks(state);
CREATE INDEX tasks_due_date_idx      ON tasks(due_date);
CREATE INDEX tasks_priority_idx      ON tasks(priority);
CREATE INDEX tasks_staged_order_idx  ON tasks(staged_order) WHERE staged_order IS NOT NULL;
CREATE INDEX tasks_spawned_by_idx    ON tasks(spawned_by_script_id);
CREATE INDEX tasks_project_idx       ON tasks(project);

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
