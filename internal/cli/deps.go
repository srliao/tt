package cli

import "github.com/spf13/cobra"

func newDepsCmd(s *session) *cobra.Command {
	return group("deps", "Read and change task dependencies")
}
