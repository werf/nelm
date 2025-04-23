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

func newRepoLoginCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	registryCmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "registry")
	}))

	cmd := lo.Must(lo.Find(registryCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "login")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Short = "Log in to an OCI registry with charts."
	cmd.Long = ""
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 30, repoCmdGroup)

	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helm_v3.Settings

		ctx = action.SetupLogging(ctx, lo.Ternary(helmSettings.Debug, action.DebugLogLevel, action.InfoLogLevel), "", action.LogColorModeAuto)

		secrets.DisableSecrets = true
		loader.NoChartLockWarning = ""

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		return nil
	}

	return cmd
}
