package main

import (
	"cmp"
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/v2/pkg/action"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/log"
)

type chartUploadConfig struct {
	action.ChartUploadOptions

	LogColorMode string
	LogLevel     string
	LogWidth     int
}

func newChartUploadCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartUploadConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"upload [options...] chart-archive remote",
		"Upload a chart archive to a repository.",
		"Upload a chart archive to the OCI repository REMOTE/<chart name>, where REMOTE is oci://HOST[/PATH], under the chart version and the --tag tags, with a \"+\" in a tag pushed as \"_\". The provenance file <chart-archive>.prov is uploaded with the chart if it exists. A tag equal to the tag of an image from the helm.sh/images annotation of the chart in the same repository is refused before anything is uploaded. Loopback registries, like localhost:5000, are reached over plain HTTP unless TLS options are given.",
		40,
		chartCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.ExactArgs(2),
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = action.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), action.DefaultChartUploadLogLevel), action.SetupLoggingOptions{
				ColorMode: cfg.LogColorMode,
				LogWidth:  cfg.LogWidth,
			})

			if _, err := action.ChartUpload(ctx, args[0], args[1], cfg.ChartUploadOptions); err != nil {
				return fmt.Errorf("chart upload: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.ChartRepoBasicAuthPassword, "chart-repo-basic-password", "", "Basic auth password to authenticate in chart repository", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoBasicAuthUsername, "chart-repo-basic-username", "", "Basic auth username to authenticate in chart repository", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoCAPath, "chart-repo-ca", "", "Path to TLS CA file for connecting to chart repository", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoCertPath, "chart-repo-cert", "", "Path to TLS client cert file for connecting to chart repository", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoInsecure, "insecure-chart-repos", false, "Allow insecure HTTP connections to chart repository", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoKeyPath, "chart-repo-key", "", "Path to TLS client key file for connecting to chart repository", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoRequestTimeout, "chart-repo-request-timeout", 0, "Set timeout for all requests to chart repository. A request includes its whole body, so the timeout also bounds a whole blob download or upload", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoSkipTLSVerify, "no-verify-chart-repos-tls", false, "Don't verify TLS certificates of chart repository", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.RegistryCredentialsPath, "oci-chart-repos-creds", "", "Credentials to access OCI chart repositories", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraTags, "tag", []string{}, "Additional tag pointing to the uploaded chart, besides its version tag", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			NoSplitOnCommas:      true,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", common.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(action.DefaultChartUploadLogLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogWidth, "log-width", 0, "Set log width. 0 to detect automatically", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
