package cli

import "github.com/spf13/cobra"

func newStageCmd(s *session) *cobra.Command {
	return group("stage", "Work the stage (the focused batch of tasks)")
}
