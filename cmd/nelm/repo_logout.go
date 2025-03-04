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

func newRepoLogoutCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	registryCmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "registry")
	}))

	cmd := lo.Must(lo.Find(registryCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "logout")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Short = "Log out from an OCI registry with charts."
	cmd.Long = ""
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
