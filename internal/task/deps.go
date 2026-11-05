package task

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
)

// Dependency rule names carried by RuleError.Rule.
const (
	RuleBlockedTaskNotOpen = "blocked_task_not_open"
	RuleCycle              = "cycle"
	RuleBlocked            = "blocked"
	RuleHasOpenDependents  = "has_open_dependents"
)

// MaxDepBatch caps the operations (adds plus removes) in one ApplyDeps call.
const MaxDepBatch = 500

// RuleError reports a write rejected by a dependency rule. Details carries
// the ids that explain the rejection (path, blockers or dependents).
type RuleError struct {
	Rule    string
	Details map[string]any
}

func (e *RuleError) Error() string {
	switch e.Rule {
	case RuleBlockedTaskNotOpen:
		return "task: only a not_done task can gain a blocker"
	case RuleCycle:
		return "task: dependency would create a cycle"
	case RuleBlocked:
		return "task: task is blocked by unfinished tasks"
	case RuleHasOpenDependents:
		return "task: task blocks not_done tasks"
	}
	return "task: dependency rule " + e.Rule + " violated"
}

// Edge records that TaskID depends on DependsOnID. It travels as the JSON
// pair [task_id, depends_on_id].
type Edge struct {
	TaskID      int64
	DependsOnID int64
}

// MarshalJSON encodes the edge as [task_id, depends_on_id].
func (e Edge) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]int64{e.TaskID, e.DependsOnID})
}

// UnmarshalJSON decodes [task_id, depends_on_id], rejecting any other length.
func (e *Edge) UnmarshalJSON(b []byte) error {
	var pair []int64
	if err := json.Unmarshal(b, &pair); err != nil {
		return fmt.Errorf("task: edge: %w", err)
	}
	if len(pair) != 2 {
		return errors.New("task: edge must be [task_id, depends_on_id]")
	}
	e.TaskID, e.DependsOnID = pair[0], pair[1]
	return nil
}

// DepBatch parameterises ApplyDeps. Removals apply before additions.
type DepBatch struct {
	Add    []Edge `json:"add"`
	Remove []Edge `json:"remove"`
}

// Direction selects which side of a task Graph walks.
type Direction string

const (
	// DirUp walks blockers and their blockers.
	DirUp Direction = "up"
	// DirDown walks tasks that depend on the start task, transitively.
	DirDown Direction = "down"
	// DirBoth is the union of DirUp and DirDown.
	DirBoth Direction = "both"
)

// IsValid reports whether d is one of the recognized Direction constants.
func (d Direction) IsValid() bool {
	switch d {
	case DirUp, DirDown, DirBoth:
		return true
	}
	return false
}

// Graph is the subgraph reachable from a task, start task included. Tasks
// are sorted by id; Edges by (task_id, depends_on_id).
type Graph struct {
	Tasks []Task `json:"tasks"`
	Edges []Edge `json:"edges"`
}

// AddDep records that taskID depends on dependsOnID. Re-adding an existing
// edge is a no-op.
func (s *Impl) AddDep(ctx context.Context, taskID, dependsOnID int64) (Task, error) {
	err := s.inTx(ctx, "add dep", func(q *sqlcgen.Queries) error {
		g, err := loadDepGraph(ctx, q)
		if err != nil {
			return err
		}
		return addDep(ctx, q, g, Edge{TaskID: taskID, DependsOnID: dependsOnID}, s.now.Stamp())
	})
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, taskID)
}

// RemoveDep deletes the edge if present; a missing edge is a no-op.
func (s *Impl) RemoveDep(ctx context.Context, taskID, dependsOnID int64) (Task, error) {
	err := s.inTx(ctx, "remove dep", func(q *sqlcgen.Queries) error {
		return removeDep(ctx, q, Edge{TaskID: taskID, DependsOnID: dependsOnID})
	})
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, taskID)
}

// ApplyDeps applies every removal, then every addition, in one transaction:
// the batch succeeds or fails as a whole. Cycle checks run against the graph
// as it stands after the earlier operations, so swapping a blocker works. A
// RuleError from an addition gains "op", "index" and "pair" details. Returns
// every task named in the batch, sorted by id.
func (s *Impl) ApplyDeps(ctx context.Context, in DepBatch) ([]Task, error) {
	if n := len(in.Add) + len(in.Remove); n > MaxDepBatch {
		return nil, fmt.Errorf("task: batch must be at most %d operations, got %d", MaxDepBatch, n)
	}
	err := s.inTx(ctx, "apply deps", func(q *sqlcgen.Queries) error {
		g, err := loadDepGraph(ctx, q)
		if err != nil {
			return err
		}
		for _, e := range in.Remove {
			if err := removeDep(ctx, q, e); err != nil {
				return err
			}
			g.remove(e)
		}
		now := s.now.Stamp()
		for i, e := range in.Add {
			if err := addDep(ctx, q, g, e, now); err != nil {
				var re *RuleError
				if errors.As(err, &re) {
					re.Details["op"] = "add"
					re.Details["index"] = i
					re.Details["pair"] = e
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var ids []int64
	for _, e := range slices.Concat(in.Remove, in.Add) {
		ids = append(ids, e.TaskID, e.DependsOnID)
	}
	slices.Sort(ids)
	return s.getMany(ctx, slices.Compact(ids))
}

// Graph returns every task linked to id in direction dir, plus the edges
// walked to reach them.
func (s *Impl) Graph(ctx context.Context, id int64, dir Direction) (Graph, error) {
	if !dir.IsValid() {
		return Graph{}, fmt.Errorf("task: invalid direction %q", dir)
	}
	if _, err := s.q.GetTask(ctx, id); err != nil {
		return Graph{}, fmt.Errorf("task: graph %d: %w", id, err)
	}
	g, err := loadDepGraph(ctx, s.q)
	if err != nil {
		return Graph{}, err
	}

	seen := map[int64]bool{id: true}
	edges := map[Edge]bool{}
	walk := func(next map[int64][]int64, edgeOf func(from, to int64) Edge) {
		queue := []int64{id}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, n := range next[cur] {
				edges[edgeOf(cur, n)] = true
				if !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
	}
	if dir == DirUp || dir == DirBoth {
		walk(g.blockers, func(from, to int64) Edge { return Edge{TaskID: from, DependsOnID: to} })
	}
	if dir == DirDown || dir == DirBoth {
		walk(g.dependents, func(from, to int64) Edge { return Edge{TaskID: to, DependsOnID: from} })
	}

	ids := make([]int64, 0, len(seen))
	for n := range seen {
		ids = append(ids, n)
	}
	slices.Sort(ids)
	tasks, err := s.getMany(ctx, ids)
	if err != nil {
		return Graph{}, err
	}
	out := Graph{Tasks: tasks, Edges: make([]Edge, 0, len(edges))}
	for e := range edges {
		out.Edges = append(out.Edges, e)
	}
	slices.SortFunc(out.Edges, compareEdges)
	return out, nil
}

// ListDeps returns every edge sorted by (task_id, depends_on_id).
func (s *Impl) ListDeps(ctx context.Context) ([]Edge, error) {
	rows, err := s.q.ListTaskDeps(ctx)
	if err != nil {
		return nil, fmt.Errorf("task: list deps: %w", err)
	}
	out := make([]Edge, 0, len(rows))
	for _, r := range rows {
		out = append(out, Edge{TaskID: r.TaskID, DependsOnID: r.DependsOnID})
	}
	return out, nil
}

func compareEdges(a, b Edge) int {
	return cmp.Or(cmp.Compare(a.TaskID, b.TaskID), cmp.Compare(a.DependsOnID, b.DependsOnID))
}

type depGraph struct {
	blockers   map[int64][]int64
	dependents map[int64][]int64
}

func loadDepGraph(ctx context.Context, q *sqlcgen.Queries) (*depGraph, error) {
	rows, err := q.ListTaskDeps(ctx)
	if err != nil {
		return nil, fmt.Errorf("task: load dep graph: %w", err)
	}
	g := &depGraph{blockers: map[int64][]int64{}, dependents: map[int64][]int64{}}
	for _, r := range rows {
		g.add(Edge{TaskID: r.TaskID, DependsOnID: r.DependsOnID})
	}
	return g, nil
}

func (g *depGraph) has(e Edge) bool {
	return slices.Contains(g.blockers[e.TaskID], e.DependsOnID)
}

func (g *depGraph) add(e Edge) {
	g.blockers[e.TaskID] = append(g.blockers[e.TaskID], e.DependsOnID)
	g.dependents[e.DependsOnID] = append(g.dependents[e.DependsOnID], e.TaskID)
}

func (g *depGraph) remove(e Edge) {
	g.blockers[e.TaskID] = slices.DeleteFunc(g.blockers[e.TaskID], func(n int64) bool { return n == e.DependsOnID })
	g.dependents[e.DependsOnID] = slices.DeleteFunc(g.dependents[e.DependsOnID], func(n int64) bool { return n == e.TaskID })
}

func (g *depGraph) pathTo(from, to int64) []int64 {
	prev := map[int64]int64{from: from}
	queue := []int64{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == to {
			path := []int64{cur}
			for cur != from {
				cur = prev[cur]
				path = append(path, cur)
			}
			slices.Reverse(path)
			return path
		}
		for _, n := range g.blockers[cur] {
			if _, ok := prev[n]; !ok {
				prev[n] = cur
				queue = append(queue, n)
			}
		}
	}
	return nil
}

func addDep(ctx context.Context, q *sqlcgen.Queries, g *depGraph, e Edge, now string) error {
	blocked, _, err := loadEdgeTasks(ctx, q, e)
	if err != nil {
		return err
	}
	if g.has(e) {
		return nil
	}
	if State(blocked.State) != StateNotDone {
		return &RuleError{Rule: RuleBlockedTaskNotOpen, Details: map[string]any{"task_id": e.TaskID}}
	}
	if path := g.pathTo(e.DependsOnID, e.TaskID); path != nil {
		return &RuleError{Rule: RuleCycle, Details: map[string]any{"path": append([]int64{e.TaskID}, path...)}}
	}
	if err := q.AddTaskDep(ctx, sqlcgen.AddTaskDepParams{TaskID: e.TaskID, DependsOnID: e.DependsOnID, CreatedAt: now}); err != nil {
		return fmt.Errorf("task: add dep %d→%d: %w", e.TaskID, e.DependsOnID, err)
	}
	g.add(e)
	return nil
}

func removeDep(ctx context.Context, q *sqlcgen.Queries, e Edge) error {
	if _, _, err := loadEdgeTasks(ctx, q, e); err != nil {
		return err
	}
	if err := q.DeleteTaskDep(ctx, sqlcgen.DeleteTaskDepParams{TaskID: e.TaskID, DependsOnID: e.DependsOnID}); err != nil {
		return fmt.Errorf("task: remove dep %d→%d: %w", e.TaskID, e.DependsOnID, err)
	}
	return nil
}

func loadEdgeTasks(ctx context.Context, q *sqlcgen.Queries, e Edge) (dependent, blocker sqlcgen.Task, err error) {
	dependent, err = q.GetTask(ctx, e.TaskID)
	if err != nil {
		return dependent, blocker, fmt.Errorf("task: dep task %d: %w", e.TaskID, err)
	}
	blocker, err = q.GetTask(ctx, e.DependsOnID)
	if err != nil {
		return dependent, blocker, fmt.Errorf("task: dep blocker %d: %w", e.DependsOnID, err)
	}
	return dependent, blocker, nil
}

func (s *Impl) loadDeps(ctx context.Context, ts []Task) error {
	if len(ts) == 0 {
		return nil
	}
	ids := make([]int64, len(ts))
	index := make(map[int64]int, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
		index[t.ID] = i
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("task: encode dep ids: %w", err)
	}
	rows, err := s.q.ListTaskDepsTouching(ctx, string(idsJSON))
	if err != nil {
		return fmt.Errorf("task: load task deps: %w", err)
	}
	for _, r := range rows {
		if i, ok := index[r.TaskID]; ok {
			ts[i].BlockedBy = append(ts[i].BlockedBy, r.DependsOnID)
			if State(r.DependsOnState) != StateDone {
				ts[i].Blocked = true
			}
		}
		if i, ok := index[r.DependsOnID]; ok {
			ts[i].Blocks = append(ts[i].Blocks, r.TaskID)
		}
	}
	return nil
}
