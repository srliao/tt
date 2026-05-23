# Phase 2 — Backend: parse `tag_filter`, Any/All, Untagged

## Goal

Make `GET /tasks` honor the new `tag_filter` param. Implement Any/All
intersection logic and Untagged (zero-tag) filtering in SQL. Accept the
legacy repeated `tag=` param as read-only back-compat (treated as `any:`).

## Files touched

- `internal/httpapi/tasks_handler.go` — parse `tag_filter`, keep `tag=` reader
- `internal/task/service.go` — `TagFilter` struct on `ListTasks` params
- `internal/db/queries/tasks.sql` — rewrite the tag join for Any/All/Untagged

## Contract

```go
// internal/task/service.go

type TagMatchMode string

const (
    TagMatchAny TagMatchMode = "any"
    TagMatchAll TagMatchMode = "all"
)

// UntaggedToken is the reserved name in TagFilter.Names that selects
// tasks with zero rows in task_tags.
const UntaggedToken = "@untagged"

type TagFilter struct {
    Mode  TagMatchMode
    Names []string  // may contain UntaggedToken
}

type ListTasksParams struct {
    States []TaskState
    Tags   TagFilter   // zero value (Mode "", Names nil) = no tag filter
    Due    TaskDueRange
    Q      string
    Sort   TaskSortAxis
    Asc    bool
    Limit  int
    Offset int
}
```

## Code patterns

### HTTP handler — parse and back-compat

```go
// internal/httpapi/tasks_handler.go

func parseTagFilter(q url.Values) task.TagFilter {
    // Preferred form: ?tag_filter=any:work,errand
    if raw := q.Get("tag_filter"); raw != "" {
        idx := strings.IndexByte(raw, ':')
        if idx < 0 {
            return task.TagFilter{}
        }
        mode := task.TagMatchMode(raw[:idx])
        if mode != task.TagMatchAny && mode != task.TagMatchAll {
            return task.TagFilter{}
        }
        rest := raw[idx+1:]
        if rest == "" {
            return task.TagFilter{}
        }
        names := strings.Split(rest, ",")
        out := names[:0]
        for _, n := range names {
            n = strings.TrimSpace(n)
            if n != "" {
                out = append(out, n)
            }
        }
        if len(out) == 0 {
            return task.TagFilter{}
        }
        return task.TagFilter{Mode: mode, Names: out}
    }
    // Legacy: ?tag=foo&tag=bar — treated as any. Removed in Phase 6.
    if legacy := q["tag"]; len(legacy) > 0 {
        return task.TagFilter{Mode: task.TagMatchAny, Names: legacy}
    }
    return task.TagFilter{}
}
```

Wire it in the existing list handler where `tags` was parsed before. Drop the
old slice path; `parseTagFilter` is now the single source.

### SQL — Any / All / Untagged

The current join (`tag IN (...)`) only handles Any. The rewrite uses three
branches off the `TagFilter` shape. Pseudocode for the query builder in
`internal/db/queries/tasks.sql` (sqlc / hand-rolled — match the repo's style):

```sql
-- Branch 1: no tag filter
-- (no JOIN, no WHERE clause for tags)

-- Branch 2: untagged only (Names == [@untagged])
SELECT t.* FROM tasks t
WHERE NOT EXISTS (SELECT 1 FROM task_tags tt WHERE tt.task_id = t.id)
  AND <other filters>;

-- Branch 3: Any with one or more real tags (and maybe @untagged)
-- Names = real_names + (untagged ? 1 : 0)
SELECT t.* FROM tasks t
WHERE (
    EXISTS (
      SELECT 1 FROM task_tags tt
      JOIN tags g ON g.id = tt.tag_id
      WHERE tt.task_id = t.id AND g.name = ANY($real_names)
    )
    OR ($include_untagged AND NOT EXISTS (
      SELECT 1 FROM task_tags tt WHERE tt.task_id = t.id
    ))
  )
  AND <other filters>;

-- Branch 4: All with one or more real tags (@untagged invalid here — UI prevents)
-- Names = real_names
SELECT t.* FROM tasks t
WHERE (
    SELECT COUNT(DISTINCT g.name) FROM task_tags tt
    JOIN tags g ON g.id = tt.tag_id
    WHERE tt.task_id = t.id AND g.name = ANY($real_names)
  ) = $real_count
  AND <other filters>;
```

Branch 4's `COUNT(DISTINCT g.name) = $real_count` is the standard pattern for
"has all of N". Index recommendation: `(task_tags.task_id, task_tags.tag_id)`
exists already. Add a partial index on `tags(name)` if EXPLAIN shows a seq
scan there.

### Service layer

```go
// internal/task/service.go

func (s *Service) ListTasks(ctx context.Context, p ListTasksParams) ([]Task, error) {
    real, includeUntagged := splitUntagged(p.Tags.Names)

    switch {
    case len(p.Tags.Names) == 0:
        // no tag filter
    case len(real) == 0 && includeUntagged:
        // Branch 2
    case p.Tags.Mode == TagMatchAll:
        // Branch 4; if includeUntagged, the result is empty by definition —
        // return immediately. The UI prevents this combination, but the
        // service must be safe to call directly.
        if includeUntagged {
            return nil, nil
        }
    default:
        // Branch 3 (Any)
    }
    // ... build and execute the query
}

func splitUntagged(names []string) (real []string, includeUntagged bool) {
    real = make([]string, 0, len(names))
    for _, n := range names {
        if n == UntaggedToken {
            includeUntagged = true
        } else {
            real = append(real, n)
        }
    }
    return
}
```

### Validation

- Tag names containing `@` are already rejected (`internal/tag/validate.go`).
  Add a regression test that confirms a name like `@foo` returns 400 from
  `POST /tags`.
- If `tag_filter=all:@untagged,work` arrives despite the UI guard, the service
  returns an empty list — not an error.

## Acceptance

- `GET /tasks?tag_filter=any:@untagged` returns only tasks with zero rows in `task_tags`.
- `GET /tasks?tag_filter=any:work,errand` returns the same set as today's
  `GET /tasks?tag=work&tag=errand`.
- `GET /tasks?tag_filter=all:work,urgent` returns only tasks tagged with both.
- `GET /tasks?tag_filter=any:@untagged,work` returns the union of untagged tasks and tasks tagged `work`.
- `GET /tasks?tag_filter=all:@untagged,work` returns an empty list (no 500).
- Legacy `GET /tasks?tag=work&tag=errand` still returns the same set as the new
  `any:work,errand` form (back-compat reader active until Phase 6).
- Malformed `tag_filter` (no colon, unknown mode, empty list) is treated as
  "no tag filter" — never a 400.

## Tests

- Service-level tests for all four branches plus `all + @untagged` → empty.
- HTTP-level tests covering: missing param, legacy `tag=`, well-formed
  `tag_filter` for each branch, malformed input.
- Validation regression: `@foo` is rejected by `POST /tags`.
- Snapshot the SQL each branch emits if the repo has a query-snapshot
  convention (see other handlers).

## Dependencies

- Phase 1 (frontend contract) — same wire format, no surprises at runtime.
