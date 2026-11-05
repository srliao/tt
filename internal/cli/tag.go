package cli

import "github.com/spf13/cobra"

func newTagCmd(s *session) *cobra.Command {
	return group("tag", "List and create tags")
}
