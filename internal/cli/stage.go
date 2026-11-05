package cli

import (
	"cmp"
	"context"
	"net/http"
	"slices"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/task"
)

func newStageCmd(s *session) *cobra.Command {
	return group("stage", "Work the stage (the hand-picked, ordered tasks being worked through now)",
		newStageListCmd(s),
		newStageReorderCmd(s),
		newStageClearFinishedCmd(s),
	)
}

// stagedTasks returns every staged task, done or not, in stage order.
func (s *session) stagedTasks(ctx context.Context) ([]task.Task, error) {
	c, err := s.api()
	if err != nil {
		return nil, err
	}
	var all []task.Task
	if _, err := c.Do(ctx, http.MethodGet, "/tasks", nil, nil, &all); err != nil {
		return nil, err
	}
	staged := slices.DeleteFunc(all, func(t task.Task) bool { return t.StagedOrder == nil })
	slices.SortStableFunc(staged, func(a, b task.Task) int { return cmp.Compare(*a.StagedOrder, *b.StagedOrder) })
	return staged, nil
}

func newStageListCmd(s *session) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "list [--full]",
		Short: "List staged tasks in stage order",
		Long: `List every staged task, including finished ones not yet cleared, in stage
order. Not paginated.

Output: {tasks: [row]}
  row (default): ` + compactTaskShape + `
  row (--full):  ` + fullTaskShape,
		Example: `  tt-cli stage list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			staged, err := s.stagedTasks(cmd.Context())
			if err != nil {
				return err
			}
			return s.out(taskList{Tasks: taskRows(staged, full)})
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "full task rows")
	return cmd
}

func newStageReorderCmd(s *session) *cobra.Command {
	var top, bottom bool
	var before, after int64
	cmd := &cobra.Command{
		Use:   "reorder <id> (--before <id> | --after <id> | --top | --bottom)",
		Short: "Move a staged task within the stage",
		Long: `Move a staged task directly before or after another staged task, or to
the top or bottom of the stage. Pass exactly one position flag.

Output: the moved task ` + fullTaskShape,
		Example: `  tt-cli stage reorder 42 --before 17`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("id", args[0])
			if err != nil {
				return err
			}
			fl := cmd.Flags()
			n := 0
			for _, name := range []string{"before", "after", "top", "bottom"} {
				if fl.Changed(name) {
					n++
				}
			}
			if n != 1 {
				return usageErr("pass exactly one of --before, --after, --top, --bottom", nil)
			}

			ctx := cmd.Context()
			staged, err := s.stagedTasks(ctx)
			if err != nil {
				return err
			}
			if err := s.requireStaged(ctx, staged, id); err != nil {
				return err
			}
			rest := slices.DeleteFunc(slices.Clone(staged), func(t task.Task) bool { return t.ID == id })
			// The server places the task between two neighbours: the one
			// that will sit above it (before_id) and below it (after_id).
			var above, below *int64
			at := func(i int) *int64 {
				if i < 0 || i >= len(rest) {
					return nil
				}
				return &rest[i].ID
			}
			switch {
			case top:
				below = at(0)
			case bottom:
				above = at(len(rest) - 1)
			default:
				anchor := before
				if fl.Changed("after") {
					anchor = after
				}
				if err := s.requireStaged(ctx, staged, anchor); err != nil {
					return err
				}
				i := slices.IndexFunc(rest, func(t task.Task) bool { return t.ID == anchor })
				if fl.Changed("before") {
					above, below = at(i-1), at(i)
				} else {
					above, below = at(i), at(i+1)
				}
			}

			c, err := s.api()
			if err != nil {
				return err
			}
			var out task.Task
			body := map[string]any{"task_id": id, "before_id": above, "after_id": below}
			if _, err := c.Do(ctx, http.MethodPost, "/stage/reorder", nil, body, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
	fl := cmd.Flags()
	fl.Int64Var(&before, "before", 0, "place directly before this staged task")
	fl.Int64Var(&after, "after", 0, "place directly after this staged task")
	fl.BoolVar(&top, "top", false, "move to the top of the stage")
	fl.BoolVar(&bottom, "bottom", false, "move to the bottom of the stage")
	return cmd
}

// requireStaged errors with not_found for a missing task and a validation
// error for a task that exists but is not staged.
func (s *session) requireStaged(ctx context.Context, staged []task.Task, id int64) error {
	if slices.ContainsFunc(staged, func(t task.Task) bool { return t.ID == id }) {
		return nil
	}
	if _, err := s.getTask(ctx, id); err != nil {
		return err
	}
	return validationErr("task is not staged", map[string]any{"id": id})
}

func newStageClearFinishedCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "clear-finished",
		Short: "Unstage every done or cancelled task",
		Long: `Remove done and cancelled tasks from the stage, keeping open ones.

Output: {ok: true}`,
		Example: `  tt-cli stage clear-finished`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := s.api()
			if err != nil {
				return err
			}
			if _, err := c.Do(cmd.Context(), http.MethodDelete, "/stage/finished", nil, nil, nil); err != nil {
				return err
			}
			return s.out(map[string]bool{"ok": true})
		},
	}
}
