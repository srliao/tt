package cli

import "github.com/spf13/cobra"

func newDoctorCmd(s *session) *cobra.Command {
	return &cobra.Command{Use: "doctor"}
}
