package commands

import (
	"github.com/spf13/cobra"
)

func NewReleaseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "release",
		Short:   "Manage Helm releases",
		Long:    "Manage Helm releases with the specified release name.",
	}

	cmd.AddCommand(NewReleaseDeployCommand())
	cmd.AddCommand(NewReleaseUninstallCommand())

	return cmd
}
