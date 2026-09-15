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

func newChartUploadCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "push")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Use = "upload [archive] [remote]"
	cmd.Short = "Upload a chart archive to a repository."
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 40, chartCmdGroup)

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
