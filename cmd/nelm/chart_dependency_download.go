package main

import (
	"context"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
)

func newChartDependencyDownloadCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	dependencyCmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "dependency")
	}))

	cmd := lo.Must(lo.Find(dependencyCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "build")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Use = "download CHART"
	cmd.Short = "Download chart dependencies from Chart.lock."
	cmd.Long = "Download chart dependencies from Chart.lock."
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 50, dependencyCmdGroup)

	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helm_v3.Settings

		ctx = action.SetupLogging(ctx, lo.Ternary(helmSettings.Debug, action.DebugLogLevel, action.InfoLogLevel), "", action.LogColorModeAuto, false)

		secrets.DisableSecrets = true
		loader.NoChartLockWarning = ""

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
