package main

import (
	"context"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/nelm/pkg/log"
)

func newChartDownloadCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "pull")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Use = "download [chart URL | repo/chartname] [...]"
	cmd.Short = "Download a chart from a repository."
	cmd.Aliases = []string{}

	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helm_v3.Settings

		if helmSettings.Debug {
			log.Default.SetLevel(ctx, log.DebugLevel)
		} else {
			log.Default.SetLevel(ctx, log.InfoLevel)
		}

		secrets.DisableSecrets = true
		loader.NoChartLockWarning = ""

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
