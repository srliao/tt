package task

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
	"github.com/srliao/tt/internal/tag"
)

// Plan creates a set of tasks and adds or removes dependencies in one
// transaction. Refs name the plan's own tasks; edge ends are a ref or an
// existing task id.
type Plan struct {
	Tasks      []PlanTask `json:"tasks"`
	Deps       []PlanEdge `json:"deps"`
	DepsRemove []PlanEdge `json:"deps_remove"`
}

// PlanTask is a task to create. Tags are names of existing tags.
type PlanTask struct {
	Ref     string   `json:"ref"`
	Title   string   `json:"title"`
	Notes   string   `json:"notes"`
	DueDate *string  `json:"due_date"`
	Tags    []string `json:"tags"`
	Stage   bool     `json:"stage"`
}

// PlanEdge records that Task depends on DependsOn.
type PlanEdge struct {
	Task      PlanRef `json:"task"`
	DependsOn PlanRef `json:"depends_on"`
}

// PlanRef is one end of a PlanEdge: a plan ref (JSON string) or an existing
// task id (JSON number).
type PlanRef struct {
	Ref string
	ID  int64
}

// MarshalJSON encodes the ref as a string, or the id as a number.
func (r PlanRef) MarshalJSON() ([]byte, error) {
	if r.Ref != "" {
		return json.Marshal(r.Ref)
	}
	return json.Marshal(r.ID)
}

// UnmarshalJSON accepts a JSON string (ref) or integer (task id).
func (r *PlanRef) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var ref string
		if err := json.Unmarshal(b, &ref); err != nil {
			return fmt.Errorf("task: plan edge ref: %w", err)
		}
		if ref == "" {
			return errors.New("task: plan edge ref must not be empty")
		}
		*r = PlanRef{Ref: ref}
		return nil
	}
	var id int64
	if err := json.Unmarshal(b, &id); err != nil {
		return errors.New("task: plan edge end must be a ref string or a task id")
	}
	*r = PlanRef{ID: id}
	return nil
}

// PlanResult maps each ref to its new task id. Tasks holds the plan's tasks
// in doc order, then any existing task an edge touched, sorted by id.
type PlanResult struct {
	IDs   map[string]int64 `json:"ids"`
	Tasks []Task           `json:"tasks"`
}

// PlanError reports a plan rejected as invalid. Details names the failing
// item. Err, when set, is the cause; sql.ErrNoRows marks an unknown task id.
type PlanError struct {
	Msg     string
	Details map[string]any
	Err     error
}

func (e *PlanError) Error() string {
	if e.Err != nil {
		return "task: plan: " + e.Msg + ": " + e.Err.Error()
	}
	return "task: plan: " + e.Msg
}

func (e *PlanError) Unwrap() error { return e.Err }

// ApplyPlan creates every plan task (with tags and stage), then removes and
// adds the plan's deps, all in one transaction. Dependency rules apply as in
// ApplyDeps; a RuleError gains "op", "index" and "edge" details. Plan tasks
// land above every existing task, in doc order.
func (s *Impl) ApplyPlan(ctx context.Context, p Plan) (PlanResult, error) {
	if err := validatePlan(&p); err != nil {
		return PlanResult{}, err
	}
	ids := make(map[string]int64, len(p.Tasks))
	err := s.inTx(ctx, "apply plan", func(q *sqlcgen.Queries) error {
		minP, err := q.MinPriority(ctx)
		if err != nil {
			return fmt.Errorf("task: plan min priority: %w", err)
		}
		minS, err := q.MinStagedOrder(ctx)
		if err != nil {
			return fmt.Errorf("task: plan min staged_order: %w", err)
		}
		priority := coerceFloat(minP) - float64(len(p.Tasks))
		stagedOrder := coerceFloat(minS) - float64(len(p.Tasks))

		tagIDs, err := tagIDsByName(ctx, q)
		if err != nil {
			return err
		}

		for i, pt := range p.Tasks {
			for _, name := range pt.Tags {
				if _, ok := tagIDs[tag.NormalizeName(name)]; !ok {
					return &PlanError{
						Msg: fmt.Sprintf("task %d (%q): unknown tag %q", i, pt.Ref, name),
						Details: map[string]any{
							"field": "tags", "index": i, "ref": pt.Ref, "tag": name,
							"existing_tags": slices.Sorted(maps.Keys(tagIDs)),
						},
					}
				}
			}
		}

		now := s.now.Stamp()
		for _, pt := range p.Tasks {
			params := sqlcgen.CreateTaskParams{
				Title:     pt.Title,
				Notes:     pt.Notes,
				DueDate:   pt.DueDate,
				Priority:  priority,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if pt.Stage {
				params.StagedOrder = &stagedOrder
				stagedOrder++
			}
			priority++
			row, err := q.CreateTask(ctx, params)
			if err != nil {
				return fmt.Errorf("task: plan create %q: %w", pt.Ref, err)
			}
			ids[pt.Ref] = row.ID
			for _, name := range pt.Tags {
				if err := q.AddTaskTag(ctx, sqlcgen.AddTaskTagParams{TaskID: row.ID, TagID: tagIDs[tag.NormalizeName(name)]}); err != nil {
					return fmt.Errorf("task: plan tag %q: %w", pt.Ref, err)
				}
			}
		}

		resolve := func(pe PlanEdge) Edge {
			return Edge{TaskID: resolveRef(ids, pe.Task), DependsOnID: resolveRef(ids, pe.DependsOn)}
		}
		g, err := loadDepGraph(ctx, q)
		if err != nil {
			return err
		}
		for i, pe := range p.DepsRemove {
			e := resolve(pe)
			if err := removeDep(ctx, q, e); err != nil {
				return planEdgeError(err, "remove", i, pe)
			}
			g.remove(e)
		}
		for i, pe := range p.Deps {
			if err := addDep(ctx, q, g, resolve(pe), now); err != nil {
				return planEdgeError(err, "add", i, pe)
			}
		}
		return nil
	})
	if err != nil {
		return PlanResult{}, err
	}

	created := make([]int64, 0, len(p.Tasks))
	for _, pt := range p.Tasks {
		created = append(created, ids[pt.Ref])
	}
	var touched []int64
	for _, pe := range slices.Concat(p.DepsRemove, p.Deps) {
		for _, r := range []PlanRef{pe.Task, pe.DependsOn} {
			if r.Ref == "" && !slices.Contains(created, r.ID) {
				touched = append(touched, r.ID)
			}
		}
	}
	slices.Sort(touched)
	tasks, err := s.getMany(ctx, slices.Concat(created, slices.Compact(touched)))
	if err != nil {
		return PlanResult{}, err
	}
	return PlanResult{IDs: ids, Tasks: tasks}, nil
}

// validatePlan checks everything that needs no database: the op cap, refs,
// titles, due dates and edge refs. It trims titles and normalizes due dates
// in place.
func validatePlan(p *Plan) error {
	if n := len(p.Tasks) + len(p.Deps) + len(p.DepsRemove); n > MaxDepBatch {
		return &PlanError{
			Msg:     fmt.Sprintf("plan must be at most %d operations, got %d", MaxDepBatch, n),
			Details: map[string]any{"max": MaxDepBatch, "got": n},
		}
	}
	refs := make(map[string]bool, len(p.Tasks))
	for i := range p.Tasks {
		pt := &p.Tasks[i]
		fail := func(field, msg string) error {
			return &PlanError{
				Msg:     fmt.Sprintf("task %d (%q): %s", i, pt.Ref, msg),
				Details: map[string]any{"field": field, "index": i, "ref": pt.Ref},
			}
		}
		if strings.TrimSpace(pt.Ref) == "" {
			return fail("ref", "ref is required")
		}
		if refs[pt.Ref] {
			return fail("ref", "duplicate ref")
		}
		refs[pt.Ref] = true
		pt.Title = strings.TrimSpace(pt.Title)
		if pt.Title == "" {
			return fail("title", "title is required")
		}
		due, err := normalizeDueDate(pt.DueDate)
		if err != nil {
			return fail("due_date", "due_date must be YYYY-MM-DD")
		}
		pt.DueDate = due
	}
	for _, list := range []struct {
		op    string
		edges []PlanEdge
	}{{"remove", p.DepsRemove}, {"add", p.Deps}} {
		for i, pe := range list.edges {
			for _, end := range []struct {
				field string
				ref   PlanRef
			}{{"task", pe.Task}, {"depends_on", pe.DependsOn}} {
				if end.ref.Ref != "" && !refs[end.ref.Ref] {
					return &PlanError{
						Msg:     fmt.Sprintf("%s edge %d: unknown ref %q", list.op, i, end.ref.Ref),
						Details: map[string]any{"field": end.field, "op": list.op, "index": i, "ref": end.ref.Ref},
					}
				}
			}
		}
	}
	return nil
}

// planEdgeError tags a failed edge operation with the edge that caused it:
// a RuleError gains the details in place, an unknown task id becomes a
// PlanError. Other errors pass through.
func planEdgeError(err error, op string, index int, pe PlanEdge) error {
	details := map[string]any{"op": op, "index": index, "edge": pe}
	var re *RuleError
	if errors.As(err, &re) {
		maps.Copy(re.Details, details)
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return &PlanError{Msg: fmt.Sprintf("%s edge %d: unknown task", op, index), Details: details, Err: err}
	}
	return err
}

func resolveRef(ids map[string]int64, r PlanRef) int64 {
	if r.Ref != "" {
		return ids[r.Ref]
	}
	return r.ID
}

func tagIDsByName(ctx context.Context, q *sqlcgen.Queries) (map[string]int64, error) {
	rows, err := q.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("task: plan list tags: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Name] = r.ID
	}
	return out, nil
}
