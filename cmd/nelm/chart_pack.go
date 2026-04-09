package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	helmcmd "github.com/werf/nelm/pkg/helm/pkg/cmd"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts"
)

func newChartPackCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	opts := common.TypeScriptOptions{}
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
		helmSettings := helmcmd.Settings

		// FIXME(major): should we do it like that everywhere, setting the context?
		ctx = action.SetupLogging(cmd.Context(), lo.Ternary(helmSettings.Debug, log.DebugLevel, log.InfoLevel), action.SetupLoggingOptions{})
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
