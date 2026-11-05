package cli

import (
	"database/sql"
	"errors"
	"maps"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/task"
)

func newPlanCmd(s *session) *cobra.Command {
	return group("plan", "Create tasks and wire their dependencies in one step",
		newPlanCheckCmd(s),
		newPlanApplyCmd(s),
	)
}

const planIO = `Input (-f file, or stdin; see ` + "`tt-cli help plans`" + `):
  {"tasks": [{ref, title, notes?, due_date?, tags?: [name], stage?: bool}],
   "deps": [{task: ref|id, depends_on: ref|id}],
   "deps_remove": [{task: ref|id, depends_on: ref|id}]}
Output: {ids: {ref: id}, tasks: [task]}
  tasks: the plan's tasks in doc order, then existing tasks an edge touched
  task: ` + fullTaskShape

func newPlanCheckCmd(s *session) *cobra.Command {
	var file, snapshot string
	cmd := &cobra.Command{
		Use:   "check [-f plan.json] [--snapshot file.db]",
		Short: "Dry-run a plan against a local copy of the server's data",
		Long: `Run the plan against an in-memory copy of the server's database, with the
same rules apply uses, and print what apply would return. Nothing is sent to
the server. The ids are provisional: apply may assign different ones.

A fresh snapshot is fetched unless --snapshot names one saved with
` + "`tt-cli snapshot pull`" + `. A saved snapshot can be stale; apply is the final check.

` + planIO,
		Example: `  tt-cli plan check -f plan.json`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := s.readPlan(file)
			if err != nil {
				return err
			}
			store, _, err := s.openSnapshot(cmd.Context(), snapshot)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			out, err := task.New(store).ApplyPlan(cmd.Context(), plan)
			if err != nil {
				return serviceErr(err)
			}
			return s.out(out)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "plan document (default stdin)")
	cmd.Flags().StringVar(&snapshot, "snapshot", "", "saved snapshot to check against (default: fetch a fresh one)")
	return cmd
}

func newPlanApplyCmd(s *session) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "apply [-f plan.json]",
		Short: "Create a plan's tasks and dependencies on the server",
		Long: `Send the plan to the server, which applies it in one transaction: either
every task and edge is written or, on any error, nothing is.

` + planIO,
		Example: `  tt-cli plan apply -f plan.json`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := s.readPlan(file)
			if err != nil {
				return err
			}
			var out task.PlanResult
			if err := s.do(cmd.Context(), http.MethodPost, "/plans", nil, plan, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "plan document (default stdin)")
	return cmd
}

func (s *session) readPlan(file string) (task.Plan, error) {
	var plan task.Plan
	raw, err := s.readInput(file)
	if err != nil {
		return plan, err
	}
	return plan, decodeDoc(raw, &plan)
}

// serviceErr maps an error from service code run locally to the code the
// server would have answered with.
func serviceErr(err error) error {
	var re *task.RuleError
	if errors.As(err, &re) {
		details := map[string]any{"rule": re.Rule}
		maps.Copy(details, re.Details)
		return &Error{Code: CodeConflict, Message: re.Error(), Details: details, Exit: ExitValidation}
	}
	var pe *task.PlanError
	if errors.As(err, &pe) {
		if errors.Is(pe, sql.ErrNoRows) {
			return &Error{Code: CodeNotFound, Message: pe.Msg, Details: pe.Details, Exit: ExitNotFound}
		}
		return validationErr(pe.Msg, pe.Details)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return &Error{Code: CodeNotFound, Message: "not found", Exit: ExitNotFound}
	}
	return &Error{Code: "internal", Message: err.Error(), Exit: ExitRemote}
}
