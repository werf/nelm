package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"

	"github.com/werf/common-go/pkg/cli"
	helm_v3 "github.com/werf/nelm/pkg/helm/cmd/helm"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts"
)

func newChartPackCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	opts := helmopts.TypeScriptOptions{}
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
		ctx = ts.NewContextWithTSOptions(ctx, opts)
		cmd.SetContext(ctx)

		loader.NoChartLockWarning = ""

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &opts.DenoBinaryPath, "deno-binary-path", "", "Path to the Deno binary to use instead of auto-downloading.", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                tsFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
