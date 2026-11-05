package cli

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/task"
)

func newDepsCmd(s *session) *cobra.Command {
	return group("deps", "Read and change task dependencies",
		newDepsAddCmd(s),
		newDepsRmCmd(s),
		newDepsBatchCmd(s),
		newDepsGraphCmd(s),
	)
}

func newDepsAddCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "add <task-id> <depends-on-id>",
		Short: "Record that a task depends on (is blocked by) another",
		Long: `Record that <task-id> depends on <depends-on-id>. Re-adding an edge is a
no-op. Cycles and other dependency rule violations fail with code conflict
and details.rule.

Output: the dependent task ` + fullTaskShape,
		Example: `  tt-cli deps add 102 101   # 102 is blocked by 101`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, dependsOn, err := parseEdge(args)
			if err != nil {
				return err
			}
			var out task.Task
			if err := s.do(cmd.Context(), http.MethodPost, fmt.Sprintf("/tasks/%d/deps", taskID), nil,
				map[string]int64{"depends_on_id": dependsOn}, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
}

func newDepsRmCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <task-id> <depends-on-id>",
		Short: "Remove a dependency edge",
		Long: `Remove the edge "<task-id> depends on <depends-on-id>".

Output: the dependent task ` + fullTaskShape,
		Example: `  tt-cli deps rm 102 101`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, dependsOn, err := parseEdge(args)
			if err != nil {
				return err
			}
			var out task.Task
			if err := s.do(cmd.Context(), http.MethodDelete, fmt.Sprintf("/tasks/%d/deps/%d", taskID, dependsOn), nil, nil, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
}

func newDepsBatchCmd(s *session) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "batch [-f file.json]",
		Short: "Add and remove many dependency edges in one transaction",
		Long: `Apply a batch of edges atomically. Each edge is [task_id, depends_on_id].
Removals run before additions. The document is read from -f, or stdin.

Input:  {"add": [[task_id, depends_on_id], ...], "remove": [[...], ...]}
Output: {tasks: [task]} with every touched task in full
  task: ` + fullTaskShape,
		Example: `  echo '{"add": [[102, 101], [103, 102]]}' | tt-cli deps batch`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := s.readInput(file)
			if err != nil {
				return err
			}
			var body task.DepBatch
			if err := decodeDoc(raw, &body); err != nil {
				return err
			}
			out, err := list[task.Task](cmd.Context(), s, http.MethodPost, "/deps/batch", nil, body)
			if err != nil {
				return err
			}
			return s.out(taskList{Tasks: taskRows(out, true)})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "batch document (default stdin)")
	return cmd
}

func newDepsGraphCmd(s *session) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "graph <task-id> [--direction up|down|both]",
		Short: "Show the dependency graph around a task",
		Long: `Walk dependencies from a task: up = its blockers, transitively; down =
tasks that depend on it, transitively; both (default) = the union.

Output: {tasks: [task], edges: [[task_id, depends_on_id]]}
  task: ` + fullTaskShape,
		Example: `  tt-cli deps graph 102 --direction up`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID("task-id", args[0])
			if err != nil {
				return err
			}
			var out task.Graph
			if err := s.do(cmd.Context(), http.MethodGet, fmt.Sprintf("/tasks/%d/graph", id),
				url.Values{"direction": {dir}}, nil, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
	cmd.Flags().StringVar(&dir, "direction", string(task.DirBoth), "up|down|both")
	return cmd
}

func parseEdge(args []string) (int64, int64, error) {
	taskID, err := parseID("task-id", args[0])
	if err != nil {
		return 0, 0, err
	}
	dependsOn, err := parseID("depends-on-id", args[1])
	if err != nil {
		return 0, 0, err
	}
	return taskID, dependsOn, nil
}
