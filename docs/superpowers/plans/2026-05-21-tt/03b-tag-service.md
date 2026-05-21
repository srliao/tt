# Phase 03b — Tag Domain Service

> Read `00-index.md` first. Commit after each task. Parallelizable with 03a (tasks) and 03c (scripts).

**Goal:** Tag CRUD plus a `Resolve(names, autoCreate)` helper used by the userscript runtime and HTTP layer to convert tag names into ids.

**Dependencies:** Phase 02.

**Parallelizable with:** 03a, 03c.

## File map

```
internal/db/queries/tags.sql        # replace stub
internal/tag/
├── types.go
├── service.go
└── service_test.go
```

## Task 1: Replace `queries/tags.sql`

**Files:** `internal/db/queries/tags.sql`

SQL is the artifact — copy verbatim:

```sql
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
```

- [ ] Run `just db-gen && go build ./...` → clean.
- [ ] Commit:
  ```bash
  git add internal/db/queries/tags.sql internal/db/sqlc/ && git commit -m "feat(db): add tag queries"
  ```

## Task 2: Domain type

**Files:** `internal/tag/types.go`

- [ ] Define `Tag { ID int64; Name string; CreatedAt time.Time }` with `json` tags.
- [ ] Verify build.
- [ ] Commit:
  ```bash
  git add internal/tag/types.go && git commit -m "feat(tag): add domain type"
  ```

## Task 3: Service — failing tests

**Files:** `internal/tag/service_test.go`

- [ ] Add a `newSvc(t)` helper using `dbtest.New(t)` and `tag.New(store)`.
- [ ] **Tests cover:**
  - `Create("work")` + `Create("home")` then `List()` returns 2 entries sorted alphabetically (`home` before `work`).
  - `Create("  spaced  ")` trims to `"spaced"`; empty / whitespace-only names error.
  - `Create("dup")` twice returns the same id both times (no duplicate-name error).
  - `Rename(id, "new")` updates the name.
  - `Delete(id)` removes; subsequent `List` is empty.
  - `Resolve([]string{"a","b","a"}, true)` auto-creates "a" and "b" once each, returns 2 unique ids.
  - `Resolve([]string{"nope"}, false)` returns an error mentioning the missing name.
- [ ] Run → undefined `tag.New`.

## Task 4: Service — implementation

**Files:** `internal/tag/service.go`

- [ ] Define `Service` interface: `Create`, `Rename`, `Delete`, `List`, `Resolve`, `GetByName`.
- [ ] `Impl` wraps `*db.Store` + `*sqlcgen.Queries`.
- [ ] `Create(name)` trims, errors on empty, calls `GetByName` first and returns the existing tag if any, otherwise inserts.
- [ ] `GetByName` returns `(nil, nil)` on `sql.ErrNoRows`.
- [ ] `Resolve(names, autoCreate)`:
  - Trim each name; skip empty; dedupe via a `map[string]struct{}`.
  - For each unique name: `GetByName`. If found, append id. If not found and `!autoCreate`, collect into `missing`. If not found and `autoCreate`, call `q.CreateTag`.
  - If `missing` is non-empty at the end, return an error like `"unknown tags: a, b"`.
- [ ] Implement `parseSqliteTime` (same as in the task package — try `"2006-01-02 15:04:05"` then RFC3339).
- [ ] Add compile-time check `var _ Service = (*Impl)(nil)`.
- [ ] Run tests → all green.
- [ ] Commit:
  ```bash
  git add internal/tag/ && git commit -m "feat(tag): add CRUD service with name resolution"
  ```

## Phase completion checklist

- [ ] `go test ./internal/tag/... -v` all pass.
- [ ] `go build ./...` clean.
- [ ] `var _ Service = (*Impl)(nil)` compiles.
