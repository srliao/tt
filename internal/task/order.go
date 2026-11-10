package task

import (
	"context"
	"errors"
	"fmt"

	sqlcgen "github.com/srliao/tt/internal/db/sqlc"
)

// OrderInput parameterises ReorderMainBatch and ReorderStageBatch. IDs is the
// desired relative order. Top and Bottom, at most one of them, place the block
// at an end of the list instead of where the first listed task sits.
type OrderInput struct {
	IDs    []int64 `json:"ids"`
	Top    bool    `json:"top,omitempty"`
	Bottom bool    `json:"bottom,omitempty"`
}

type orderAxis struct {
	name   string
	list   func(ctx context.Context, q *sqlcgen.Queries) ([]orderRow, error)
	set    func(ctx context.Context, q *sqlcgen.Queries, id int64, key float64, stamp string) error
	staged bool
}

type orderRow struct {
	id  int64
	key float64
}

var mainAxis = orderAxis{
	name: "main",
	list: func(ctx context.Context, q *sqlcgen.Queries) ([]orderRow, error) {
		rows, err := q.ListAllPrioritiesAsc(ctx)
		out := make([]orderRow, len(rows))
		for i, r := range rows {
			out[i] = orderRow{r.ID, r.Priority}
		}
		return out, err
	},
	set: func(ctx context.Context, q *sqlcgen.Queries, id int64, key float64, stamp string) error {
		_, err := q.SetTaskPriority(ctx, sqlcgen.SetTaskPriorityParams{Priority: key, UpdatedAt: stamp, ID: id})
		return err
	},
}

var stageAxis = orderAxis{
	name:   "stage",
	staged: true,
	list: func(ctx context.Context, q *sqlcgen.Queries) ([]orderRow, error) {
		rows, err := q.ListAllStagedAsc(ctx)
		out := make([]orderRow, len(rows))
		for i, r := range rows {
			out[i] = orderRow{r.ID, *r.StagedOrder}
		}
		return out, err
	},
	set: func(ctx context.Context, q *sqlcgen.Queries, id int64, key float64, stamp string) error {
		_, err := q.SetTaskStaged(ctx, sqlcgen.SetTaskStagedParams{StagedOrder: &key, UpdatedAt: stamp, ID: id})
		return err
	},
}

// ReorderMainBatch places the listed tasks as one contiguous block, in the
// listed order, in the main list. The call is atomic: a bad id changes nothing.
// Unlisted tasks keep their relative order. Returns the listed tasks in the
// requested order.
func (s *Impl) ReorderMainBatch(ctx context.Context, in OrderInput) ([]Task, error) {
	return s.orderBatch(ctx, mainAxis, in)
}

// ReorderStageBatch is ReorderMainBatch for the stage; every id must be staged.
func (s *Impl) ReorderStageBatch(ctx context.Context, in OrderInput) ([]Task, error) {
	return s.orderBatch(ctx, stageAxis, in)
}

func (s *Impl) orderBatch(ctx context.Context, ax orderAxis, in OrderInput) ([]Task, error) {
	if len(in.IDs) == 0 {
		return nil, errors.New("task: ids must be non-empty")
	}
	if in.Top && in.Bottom {
		return nil, errors.New("task: top and bottom must be exclusive")
	}
	seen := make(map[int64]bool, len(in.IDs))
	for _, id := range in.IDs {
		if seen[id] {
			return nil, fmt.Errorf("task: ids must be unique, %d repeated", id)
		}
		seen[id] = true
	}

	err := s.inTx(ctx, "reorder "+ax.name+" batch", func(q *sqlcgen.Queries) error {
		for _, id := range in.IDs {
			row, err := q.GetTask(ctx, id)
			if err != nil {
				return fmt.Errorf("task: order %d: %w", id, err)
			}
			if ax.staged && row.StagedOrder == nil {
				return fmt.Errorf("task: task %d is not staged", id)
			}
		}
		all, err := ax.list(ctx, q)
		if err != nil {
			return fmt.Errorf("task: order list: %w", err)
		}
		var rest []orderRow
		at := -1
		for _, r := range all {
			if r.id == in.IDs[0] {
				at = len(rest)
			}
			if seen[r.id] {
				continue
			}
			rest = append(rest, r)
		}
		switch {
		case in.Top:
			at = 0
		case in.Bottom:
			at = len(rest)
		}

		now := s.now.Stamp()
		n := len(in.IDs)
		keys := blockKeys(rest, at, n)
		if keys == nil {
			for i := range rest {
				rest[i].key = float64(i)
				if err := ax.set(ctx, q, rest[i].id, rest[i].key, now); err != nil {
					return fmt.Errorf("task: order rebalance %d: %w", rest[i].id, err)
				}
			}
			keys = blockKeys(rest, at, n)
		}
		for i, id := range in.IDs {
			if err := ax.set(ctx, q, id, keys[i], now); err != nil {
				return fmt.Errorf("task: order set %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.getMany(ctx, in.IDs)
}

func blockKeys(rest []orderRow, at, n int) []float64 {
	var before, after *float64
	if at > 0 {
		before = &rest[at-1].key
	}
	if at < len(rest) {
		after = &rest[at].key
	}
	keys := make([]float64, n)
	switch {
	case before != nil && after != nil:
		step := (*after - *before) / float64(n+1)
		if step < rebalanceEpsilon {
			return nil
		}
		for i := range keys {
			keys[i] = *before + step*float64(i+1)
		}
	case before != nil:
		for i := range keys {
			keys[i] = *before + float64(i+1)
		}
	case after != nil:
		for i := range keys {
			keys[i] = *after - float64(n-i)
		}
	default:
		for i := range keys {
			keys[i] = float64(i)
		}
	}
	return keys
}
