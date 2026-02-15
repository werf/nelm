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
		"Create plan and/or review upcoming release changes.",
		"Create plan and/or review upcoming release changes.",
		releaseCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newReleasePlanInstallCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newReleasePlanShowCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
