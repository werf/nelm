package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
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
	cmd.AddCommand(newReleaseListCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newReleaseGetCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newPlanCommand(ctx, afterAllCommandsBuiltFuncs))

	// FIXME(major): in v1 move all internal to pkg too to minimize diffs
	// FIXME(major): in v1 don't fail on unknown env var, maybe warning (which can be disabled)?

	return cmd
}
