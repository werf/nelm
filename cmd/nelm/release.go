package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/featgate"
)

func newReleaseCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"release",
		"Manage Helm releases.",
		"Manage Helm releases.",
		releaseCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newReleaseInstallCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newReleaseRollbackCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newReleaseUninstallCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newReleaseHistoryCommand(ctx, afterAllCommandsBuiltFuncs))

	if featgate.FeatGateNativeReleaseList.Enabled() {
		cmd.AddCommand(newReleaseListCommand(ctx, afterAllCommandsBuiltFuncs))
	} else {
		cmd.AddCommand(newLegacyReleaseListCommand(ctx, afterAllCommandsBuiltFuncs))
	}

	cmd.AddCommand(newReleaseGetCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newPlanCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
