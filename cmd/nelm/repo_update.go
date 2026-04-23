package main

import (
	"context"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	helmcmd "github.com/werf/nelm/pkg/helm/pkg/cmd"
	"github.com/werf/nelm/pkg/log"
)

func newRepoUpdateCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	repoCmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "repo")
	}))

	cmd := lo.Must(lo.Find(repoCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "update")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Short = "Update info about available charts for all chart repositories."
	cmd.Long = ""
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 40, repoCmdGroup)

	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helmcmd.Settings

		ctx = action.SetupLogging(ctx, lo.Ternary(helmSettings.Debug, log.DebugLevel, log.InfoLevel), action.SetupLoggingOptions{})

		loader.NoChartLockWarning = ""

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
