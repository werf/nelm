package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/featgate"
	helm_v3 "github.com/werf/nelm/pkg/helm/cmd/helm"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts"
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

		// FIXME(major): should we do it like that everywhere, setting the context?
		ctx := log.SetupLogging(cmd.Context(), lo.Ternary(helmSettings.Debug, log.DebugLevel, log.InfoLevel), log.SetupLoggingOptions{})
		cmd.SetContext(ctx)

		loader.NoChartLockWarning = ""

		if featgate.FeatGateTypescript.Enabled() {
			for _, chartPath := range args {
				if err := ts.BuildVendorBundleToDir(ctx, chartPath); err != nil {
					return fmt.Errorf("build TypeScript vendor bundle in %q: %w", chartPath, err)
				}
			}
		}

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
