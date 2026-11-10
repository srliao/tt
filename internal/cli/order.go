package cli

import (
	"net/http"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/task"
)

func newOrderCmd(s *session, group, path, what string) *cobra.Command {
	var file string
	var top, bottom bool
	cmd := &cobra.Command{
		Use:   "order [-f file.json] [--top | --bottom]",
		Short: "Reorder many tasks at once in " + what,
		Long: `Place the listed tasks as one block, in the listed order, in ` + what + `. The block
sits where the first listed task currently is, or at the top or bottom with
--top / --bottom. Unlisted tasks keep their relative order. All or nothing: a
bad id changes nothing. The document is read from -f, or stdin.

Input:  {"ids": [task_id, ...], "top": bool, "bottom": bool}  (top and bottom optional)
Output: {tasks: [task]} with the listed tasks in the new order
  task: ` + fullTaskShape,
		Example: `  echo '{"ids": [12, 7, 40, 3]}' | tt-cli ` + group + ` order --top`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := s.readInput(file)
			if err != nil {
				return err
			}
			var body task.OrderInput
			if err := decodeDoc(raw, &body); err != nil {
				return err
			}
			body.Top = body.Top || top
			body.Bottom = body.Bottom || bottom
			out, err := list[task.Task](cmd.Context(), s, http.MethodPost, path, nil, body)
			if err != nil {
				return err
			}
			return s.out(taskList{Tasks: taskRows(out, true)})
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&file, "file", "f", "", "order document (default stdin)")
	fl.BoolVar(&top, "top", false, "place the block at the top")
	fl.BoolVar(&bottom, "bottom", false, "place the block at the bottom")
	return cmd
}
