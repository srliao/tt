-- +goose Up

CREATE TABLE scripts (
  id              INTEGER PRIMARY KEY,
  name            TEXT NOT NULL,
  code            TEXT NOT NULL,
  enabled         INTEGER NOT NULL DEFAULT 1,
  schedule_kind   TEXT NOT NULL
                     CHECK (schedule_kind IN ('every_tick','daily','weekly','monthly')),
  schedule_config TEXT NOT NULL DEFAULT '{}',
  user_state      TEXT NOT NULL DEFAULT '{}',
  last_run_at     TEXT,
  created_at      TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX scripts_enabled_idx ON scripts(enabled);

CREATE TABLE tasks (
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
  updated_at           TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX tasks_state_idx         ON tasks(state);
CREATE INDEX tasks_due_date_idx      ON tasks(due_date);
CREATE INDEX tasks_priority_idx      ON tasks(priority);
CREATE INDEX tasks_staged_order_idx  ON tasks(staged_order) WHERE staged_order IS NOT NULL;
CREATE INDEX tasks_spawned_by_idx    ON tasks(spawned_by_script_id);

CREATE TABLE tags (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE task_tags (
  task_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  tag_id   INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
  PRIMARY KEY (task_id, tag_id)
);

CREATE INDEX task_tags_tag_idx ON task_tags(tag_id);

CREATE TABLE script_runs (
  id                INTEGER PRIMARY KEY,
  script_id         INTEGER NOT NULL REFERENCES scripts(id) ON DELETE CASCADE,
  started_at        TEXT NOT NULL DEFAULT (datetime('now')),
  finished_at       TEXT,
  status            TEXT NOT NULL DEFAULT 'running'
                       CHECK (status IN ('running','ok','error','timeout')),
  error_message     TEXT,
  spawned_task_ids  TEXT NOT NULL DEFAULT '[]',
  trigger           TEXT NOT NULL
                       CHECK (trigger IN ('scheduled','manual'))
);

CREATE INDEX script_runs_script_idx     ON script_runs(script_id, started_at DESC);
CREATE INDEX script_runs_started_at_idx ON script_runs(started_at DESC);

CREATE TABLE script_logs (
  id             INTEGER PRIMARY KEY,
  script_run_id  INTEGER NOT NULL REFERENCES script_runs(id) ON DELETE CASCADE,
  level          TEXT NOT NULL DEFAULT 'info'
                    CHECK (level IN ('debug','info','warn','error')),
  message        TEXT NOT NULL,
  logged_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX script_logs_run_idx ON script_logs(script_run_id, logged_at);

-- +goose Down

DROP TABLE IF EXISTS script_logs;
DROP TABLE IF EXISTS script_runs;
DROP TABLE IF EXISTS task_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS scripts;
