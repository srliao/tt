-- +goose Up

-- Excalidraw canvases. scene holds the serialized Excalidraw scene
-- ({elements, appState, files}) as opaque JSON; the server never inspects it.
-- Embedded images live inside scene.files as data URLs, so rows can be large.
CREATE TABLE drawings (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  scene      TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down

DROP TABLE drawings;
