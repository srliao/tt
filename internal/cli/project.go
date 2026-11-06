package cli

import (
	"net/http"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/task"
)

func newProjectCmd(s *session) *cobra.Command {
	return group("project", "List, set and rename projects",
		newProjectListCmd(s),
		newProjectSetCmd(s, true),
		newProjectSetCmd(s, false),
		newProjectRenameCmd(s),
	)
}

const projectShape = `{name, open_count, total_count}`

func newProjectListCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every project with its task counts",
		Long: `List every project some task names, sorted by name. open_count counts
not_done tasks; total_count counts tasks in any state.

Output: {projects: [` + projectShape + `]}`,
		Example: `  tt-cli project list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := list[task.Project](cmd.Context(), s, http.MethodGet, "/projects", nil, nil)
			if err != nil {
				return err
			}
			return s.out(map[string]any{"projects": out})
		},
	}
}

func newProjectSetCmd(s *session, set bool) *cobra.Command {
	var full bool
	use, short, minArgs := "set <name> <id>...", "Put tasks in a project", 2
	long := `Put every listed task in project <name>. The project need not exist yet.
Names are trimmed and lowercased; names starting with @ or containing a comma
are rejected. All-or-nothing: an unknown id changes no task.`
	example := `  tt-cli project set home 42 43`
	if !set {
		use, short, minArgs = "clear <id>...", "Remove tasks from their project", 1
		long = `Remove every listed task from its project. All-or-nothing: an unknown id
changes no task.`
		example = `  tt-cli project clear 42 43`
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long: long + `

Output: {tasks: [row]}
  row (default): ` + compactTaskShape + `
  row (--full):  ` + fullTaskShape,
		Example: example,
		Args:    cobra.MinimumNArgs(minArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			var project *string
			if set {
				project = &args[0]
				args = args[1:]
			}
			ids := make([]int64, 0, len(args))
			for _, a := range args {
				id, err := parseID("id", a)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			got, err := list[task.Task](cmd.Context(), s, http.MethodPost, "/tasks/project", nil,
				map[string]any{"ids": ids, "project": project})
			if err != nil {
				return err
			}
			return s.out(taskList{Tasks: taskRows(got, full)})
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "full task rows (notes, timestamps, edges)")
	return cmd
}

func newProjectRenameCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <from> <to>",
		Short: "Rename a project, merging if <to> exists",
		Long: `Move every task in project <from> to <to>. If <to> is already in use the
two projects merge. <from> must be in use.

Output: ` + projectShape + ` for <to> after the move`,
		Example: `  tt-cli project rename house home`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var out task.Project
			if err := s.do(cmd.Context(), http.MethodPost, "/projects/rename", nil,
				map[string]string{"from": args[0], "to": args[1]}, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
}
