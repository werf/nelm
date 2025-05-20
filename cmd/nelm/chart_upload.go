package main

import (
	"context"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
)

func newChartUploadCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "push")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Use = "upload [chart] [remote]"
	cmd.Short = "Upload a chart to a repository."
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 40, chartCmdGroup)

	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helm_v3.Settings

		ctx = action.SetupLogging(ctx, lo.Ternary(helmSettings.Debug, action.DebugLogLevel, action.InfoLogLevel), action.SetupLoggingOptions{})

		loader.NoChartLockWarning = ""

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
