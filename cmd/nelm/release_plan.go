package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newPlanCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"plan",
		"Show planned changes.",
		"Show planned changes.",
		releaseCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newReleasePlanInstallCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
