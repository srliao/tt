# Phase 02 — DB Layer

> Read `00-index.md` first. Commit after each task.

**Goal:** A `*db.Store` type that opens (and migrates) a SQLite database, exposes sqlc-generated typed queries via a `Queries()` accessor, and ships a reusable `dbtest.New(t)` helper.

**Dependencies:** Phase 01.

**Tech stack:** `modernc.org/sqlite` (pure-Go SQLite, no cgo), `pressly/goose/v3` (migrations), `sqlc` (codegen — install separately with `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).

**Parallelizable with:** Nothing.

## File map

```
internal/db/
├── store.go
├── store_test.go
├── dbtest/dbtest.go
├── migrations/
│   ├── 0001_init.sql
│   └── embed.go
├── queries/
│   ├── tasks.sql      (stub; populated in 03a)
│   ├── tags.sql       (stub; populated in 03b)
│   ├── scripts.sql    (stub; populated in 03c)
│   └── runs.sql       (stub; populated in 03c)
├── sqlc.yaml
└── sqlc/              (generated)
```

## Task 1: Add Go deps

- [ ] Run:
  ```bash
  cd /Users/srliao/code/tt && \
    go get modernc.org/sqlite@latest && \
    go get github.com/pressly/goose/v3@latest && \
    go mod tidy
  ```
- [ ] Verify: `grep -E 'modernc|goose' go.mod` shows both.
- [ ] Commit:
  ```bash
  git add go.mod go.sum && git commit -m "chore(db): add sqlite + goose dependencies"
  ```

## Task 2: Initial migration

**Files:** `internal/db/migrations/0001_init.sql`

This file IS the artifact — full content per spec §3 schema. Wrap inside `-- +goose Up` / `-- +goose StatementBegin` (matching `Down` at the bottom). Tables in order:

1. `scripts` (created first because `tasks.spawned_by_script_id` references it).
2. `tasks` (all columns per spec §3, including all five indexes).
3. `tags` + `task_tags` + `task_tags_tag_idx`.
4. `script_runs` (with both indexes) + `script_logs` (with one index).

Down section drops in reverse FK order: `script_logs`, `script_runs`, `task_tags`, `tags`, `tasks`, `scripts`.

- [ ] Write the file. Schema text comes verbatim from spec §3 (lines 70–170 in the spec). Wrap in goose `Up`/`Down` Statement blocks.
- [ ] Commit:
  ```bash
  git add internal/db/migrations/0001_init.sql && git commit -m "feat(db): add initial schema migration"
  ```

## Task 3: Embed migrations

**Files:** `internal/db/migrations/embed.go`

- [ ] Write:
  ```go
  package migrations

  import "embed"

  //go:embed *.sql
  var FS embed.FS
  ```
- [ ] Verify: `go build ./internal/db/migrations/...` exits 0.
- [ ] Commit:
  ```bash
  git add internal/db/migrations/embed.go && git commit -m "feat(db): embed migrations via go:embed"
  ```

## Task 4: Store — failing test

**Files:** `internal/db/store_test.go`

- [ ] Create the test file with two test functions exercising `db.Open(ctx, path) (*db.Store, error)`:
  - `TestOpenInMemoryAndMigrate` — opens `":memory:"`, queries `sqlite_master` for table names, asserts the six expected tables exist: `scripts`, `tasks`, `tags`, `task_tags`, `script_runs`, `script_logs`. Calls `store.Close()`.
  - `TestOpenFile` — opens `t.TempDir() + "/test.sqlite"`, closes cleanly.
- [ ] Run `go test ./internal/db/...`. Expected: `undefined: db.Open`.

## Task 5: Store — implementation

**Files:** `internal/db/store.go`

- [ ] Implement `Store` struct wrapping `*sql.DB`, with `DB()`, `Close()`, and the package-level `Open(ctx, path) (*Store, error)`.

Key non-obvious bits:

```go
// DSN construction — pure-Go driver wants pragmas via URI.
var dsn string
if path == ":memory:" {
    dsn = "file::memory:?cache=shared&_pragma=foreign_keys(1)"
} else {
    dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
}
sqlDB, err := sql.Open("sqlite", dsn)
// ...
if path == ":memory:" {
    sqlDB.SetMaxOpenConns(1) // so all callers share one in-memory DB
}
```

For migrations, set `goose.SetBaseFS(migrations.FS)`, `goose.SetDialect("sqlite3")`, then `goose.Up(db, ".")`. Wrap errors with `fmt.Errorf("...: %w", err)`.

- [ ] Run `go test ./internal/db/... -v` → both tests pass.
- [ ] Commit:
  ```bash
  git add internal/db/store.go internal/db/store_test.go && git commit -m "feat(db): add Store with sqlite + migrations"
  ```

## Task 6: dbtest helper

**Files:** `internal/db/dbtest/dbtest.go`

- [ ] Write a package `dbtest` exporting `New(t *testing.T) *db.Store` that opens `:memory:`, fails the test on error, registers `t.Cleanup(store.Close)`, and returns the store.
- [ ] Verify: `go build ./internal/db/...` clean.
- [ ] Commit:
  ```bash
  git add internal/db/dbtest/ && git commit -m "test(db): add dbtest helper"
  ```

## Task 7: sqlc config + stub query files

**Files:** `internal/db/sqlc.yaml`, `internal/db/queries/{tasks,tags,scripts,runs}.sql`

- [ ] Write `internal/db/sqlc.yaml` — content is the artifact:

```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "queries"
    schema: "migrations"
    gen:
      go:
        package: "sqlcgen"
        out: "sqlc"
        emit_json_tags: true
        emit_prepared_queries: false
        emit_interface: true
        emit_pointers_for_null_types: true
```

- [ ] Write stubs in each query file so sqlc has something to generate (real queries come in phase 03):

```sql
-- name: SelectTasksHealth :one
SELECT COUNT(*) FROM tasks;
```

The other three files mirror this pattern (`SelectTagsHealth`, `SelectScriptsHealth`, `SelectRunsHealth`) targeting `tags`, `scripts`, `script_runs`.

- [ ] Commit:
  ```bash
  git add internal/db/sqlc.yaml internal/db/queries/ && git commit -m "chore(db): add sqlc config and query stubs"
  ```

## Task 8: Generate initial sqlc code + just recipe

- [ ] Verify `sqlc version` works; install with `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` if missing.
- [ ] Run `cd /Users/srliao/code/tt/internal/db && sqlc generate`. Creates `sqlc/db.go`, `sqlc/models.go`, `sqlc/{tasks,tags,scripts,runs}.sql.go`.
- [ ] Verify `go build ./...` clean.
- [ ] Add to `justfile` (insert before the `clean:` recipe):

```just
# ── db codegen ───────────────────────────────────────────
db-gen:
    cd internal/db && sqlc generate
```

- [ ] Verify `just db-gen` regenerates without diff.
- [ ] Commit:
  ```bash
  git add internal/db/sqlc/ justfile && git commit -m "feat(db): generate sqlc bindings and add db-gen recipe"
  ```

## Phase completion checklist

- [ ] `go build ./...` clean.
- [ ] `go test ./internal/db/...` passes.
- [ ] `just db-gen` succeeds; rerun produces no diff.
- [ ] `internal/db/sqlc/models.go` exists.
- [ ] `dbtest.New(t)` is callable (exercised in phase 03).
