package task_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/srliao/tt/internal/task"
)

type orderAxisCase struct {
	name  string
	stage bool
}

var orderAxes = []orderAxisCase{{"main", false}, {"stage", true}}

// seedOrder creates n tasks and returns their ids in axis order, top first.
// Staged on the stage axis, so both axes start as ids[0..n-1].
func seedOrder(t *testing.T, svc *task.Impl, ctx context.Context, stage bool, n int) []int64 {
	t.Helper()
	var created []int64
	for i := 0; i < n; i++ {
		tk, err := svc.Create(ctx, task.CreateInput{Title: string(rune('a' + i))})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, tk.ID)
	}
	slices.Reverse(created)
	if stage {
		for i := n - 1; i >= 0; i-- {
			if _, err := svc.Stage(ctx, created[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	return created
}

func axisOrder(t *testing.T, svc *task.Impl, ctx context.Context, stage bool, ids []int64) []int64 {
	t.Helper()
	key := map[int64]float64{}
	for _, id := range ids {
		tk, err := svc.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		key[id] = tk.Priority
		if stage {
			key[id] = *tk.StagedOrder
		}
	}
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b int64) int {
		switch {
		case key[a] < key[b]:
			return -1
		case key[a] > key[b]:
			return 1
		}
		return 0
	})
	return out
}

func runOrder(svc *task.Impl, ctx context.Context, stage bool, in task.OrderInput) ([]task.Task, error) {
	if stage {
		return svc.ReorderStageBatch(ctx, in)
	}
	return svc.ReorderMainBatch(ctx, in)
}

func TestOrderBatch(t *testing.T) {
	cases := []struct {
		name string
		pick func(ids []int64) task.OrderInput
		want func(ids []int64) []int64
	}{
		{"partial list lands where the first listed task sat",
			func(i []int64) task.OrderInput { return task.OrderInput{IDs: []int64{i[3], i[1]}} },
			func(i []int64) []int64 { return []int64{i[0], i[3], i[1], i[2], i[4]} }},
		{"full list",
			func(i []int64) task.OrderInput { return task.OrderInput{IDs: []int64{i[4], i[3], i[2], i[1], i[0]}} },
			func(i []int64) []int64 { return []int64{i[4], i[3], i[2], i[1], i[0]} }},
		{"top",
			func(i []int64) task.OrderInput { return task.OrderInput{IDs: []int64{i[3], i[2]}, Top: true} },
			func(i []int64) []int64 { return []int64{i[3], i[2], i[0], i[1], i[4]} }},
		{"bottom",
			func(i []int64) task.OrderInput { return task.OrderInput{IDs: []int64{i[1], i[0]}, Bottom: true} },
			func(i []int64) []int64 { return []int64{i[2], i[3], i[4], i[1], i[0]} }},
		{"single id",
			func(i []int64) task.OrderInput { return task.OrderInput{IDs: []int64{i[2]}, Bottom: true} },
			func(i []int64) []int64 { return []int64{i[0], i[1], i[3], i[4], i[2]} }},
	}
	for _, ax := range orderAxes {
		for _, c := range cases {
			t.Run(ax.name+"/"+c.name, func(t *testing.T) {
				svc, ctx := newService(t)
				ids := seedOrder(t, svc, ctx, ax.stage, 5)
				in := c.pick(ids)
				got, err := runOrder(svc, ctx, ax.stage, in)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != len(in.IDs) {
					t.Fatalf("returned %d tasks, want %d", len(got), len(in.IDs))
				}
				for i, tk := range got {
					if tk.ID != in.IDs[i] {
						t.Fatalf("returned order %v, want %v", got, in.IDs)
					}
				}
				if order := axisOrder(t, svc, ctx, ax.stage, ids); !slices.Equal(order, c.want(ids)) {
					t.Fatalf("order = %v, want %v", order, c.want(ids))
				}
			})
		}
	}
}

func TestOrderBatch_BadInputWritesNothing(t *testing.T) {
	for _, ax := range orderAxes {
		t.Run(ax.name, func(t *testing.T) {
			svc, ctx := newService(t)
			ids := seedOrder(t, svc, ctx, ax.stage, 3)
			unstaged, err := svc.Create(ctx, task.CreateInput{Title: "loose"})
			if err != nil {
				t.Fatal(err)
			}
			bad := []struct {
				name string
				in   task.OrderInput
				is   error
			}{
				{"empty", task.OrderInput{}, nil},
				{"duplicate", task.OrderInput{IDs: []int64{ids[2], ids[2]}}, nil},
				{"top and bottom", task.OrderInput{IDs: ids[:1], Top: true, Bottom: true}, nil},
				{"unknown id", task.OrderInput{IDs: []int64{ids[2], 9999}}, sql.ErrNoRows},
			}
			if ax.stage {
				bad = append(bad, struct {
					name string
					in   task.OrderInput
					is   error
				}{"not staged", task.OrderInput{IDs: []int64{ids[2], unstaged.ID}}, nil})
			}
			for _, b := range bad {
				_, err := runOrder(svc, ctx, ax.stage, b.in)
				if err == nil {
					t.Fatalf("%s: no error", b.name)
				}
				if b.is != nil && !errors.Is(err, b.is) {
					t.Fatalf("%s: err = %v, want %v", b.name, err, b.is)
				}
				if order := axisOrder(t, svc, ctx, ax.stage, ids); !slices.Equal(order, ids) {
					t.Fatalf("%s: order changed to %v", b.name, order)
				}
			}
		})
	}
}

func TestOrderBatch_TightGapRebalances(t *testing.T) {
	for _, ax := range orderAxes {
		t.Run(ax.name, func(t *testing.T) {
			svc, store, ctx := newServiceWithStore(t)
			ids := seedOrder(t, svc, ctx, ax.stage, 5)
			col := map[bool]string{false: "priority", true: "staged_order"}[ax.stage]
			for i, key := range []float64{0, 1, 1.00000000005, 1.0000000001, 5} {
				if _, err := store.DB().ExecContext(ctx, "UPDATE tasks SET "+col+" = ? WHERE id = ?", key, ids[i]); err != nil {
					t.Fatal(err)
				}
			}
			// The block lands between ids[1] and ids[3], 1e-10 apart.
			if _, err := runOrder(svc, ctx, ax.stage, task.OrderInput{IDs: []int64{ids[2], ids[4]}}); err != nil {
				t.Fatal(err)
			}
			want := []int64{ids[0], ids[1], ids[2], ids[4], ids[3]}
			if order := axisOrder(t, svc, ctx, ax.stage, ids); !slices.Equal(order, want) {
				t.Fatalf("order = %v, want %v", order, want)
			}
		})
	}
}
