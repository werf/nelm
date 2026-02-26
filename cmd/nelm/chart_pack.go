package main

import (
	"context"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/werf/ts"
	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/deno"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

func newChartPackCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "package")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Use = "pack [CHART_PATH] [...]"
	cmd.Short = "Pack a chart into an archive to distribute via a repository."
	cmd.Long = strings.ReplaceAll(cmd.Long, "helm package", "nelm chart pack")
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 30, chartCmdGroup)

	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helm_v3.Settings

		ctx = log.SetupLogging(ctx, lo.Ternary(helmSettings.Debug, log.DebugLevel, log.InfoLevel), log.SetupLoggingOptions{})

		loader.NoChartLockWarning = ""

		if featgate.FeatGateTypescript.Enabled() {
			tsruntime.TSRuntime = deno.NewDenoRuntime(true)
		}

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
