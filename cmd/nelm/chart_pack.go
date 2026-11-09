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

type chartPackConfig struct {
	action.ChartPackOptions

	LogColorMode string
	LogLevel     string
	LogWidth     int
}

func newChartPackCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartPackConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"pack [options...] [chart-dir]",
		"Pack a chart into an archive to distribute via a repository.",
		"Pack a chart directory into a versioned chart archive. The publish-time inputs (values, secret values, set values, patches and default switches) are stored in .werf-data of the chart, on top of the ones the chart directory already has.",
		30,
		chartCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveFilterDirs
			},
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = action.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), action.DefaultChartPackLogLevel), action.SetupLoggingOptions{
				ColorMode: cfg.LogColorMode,
				LogWidth:  cfg.LogWidth,
			})

			chartPath := "."
			if len(args) > 0 {
				chartPath = args[0]
			}

			if _, err := action.ChartPack(ctx, chartPath, cfg.ChartPackOptions); err != nil {
				return fmt.Errorf("chart pack: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := AddValuesFlags(cmd, &cfg.ValuesOptions); err != nil {
			return fmt.Errorf("add values flags: %w", err)
		}

		if err := AddPatchesFlags(cmd, &cfg.PatchesFiles, &cfg.DefaultPatchesDisable, AddPatchesFlagsOptions{}); err != nil {
			return fmt.Errorf("add patches flags: %w", err)
		}

		if err := AddDockerConfigFlag(cmd, &cfg.DockerConfig); err != nil {
			return fmt.Errorf("add docker config flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.AppVersion, "app-version", "", "Set appVersion of Chart.yaml", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

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

		if err := cli.AddFlag(cmd, &cfg.ChartRepoSkipUpdate, "no-update-chart-repos", false, "Don't update chart repositories index", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.Name, "chart-name", "", "Set name of Chart.yaml", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.Version, "chart-version", "", "Set version of Chart.yaml", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NodeBinaryPath, "node-binary-path", "", "Path to the Node.js binary to use instead of the one in PATH.", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                tsFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.DependencyUpdate, "dependency-update", false, "Update Chart.lock and charts/ of the chart directory from Chart.yaml before packing", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.Destination, "destination", ".", "The directory to save the chart archive to", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.DefaultSecretValuesDisable, "no-default-secret-values", false, "Ignore secret-values.yaml of the top-level chart", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
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

		if err := cli.AddFlag(cmd, &cfg.ResetValues, "reset-values", false, "Drop all publish-time inputs stored in .werf-data of the chart by previous packs (values, secret values, set values, patches, default switches, release defaults, extra values, annotations, labels and images) before storing the ones of this pack", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SecretValuesFiles, "secret-values", []string{}, "Secret values files paths", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.Sign, "sign", false, "Sign the chart archive with a PGP private key and save the provenance file next to it", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SignKey, "sign-key", "", "Name of the key to sign the chart archive with", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SignKeyring, "sign-keyring", "", "Path to keyring containing the key to sign the chart archive with", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SignPassphraseFile, "sign-passphrase-file", "", `Path to file with the passphrase of the signing key, "-" to read it from stdin. Prompted for if not set`, cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, use the default system directory for temporary files", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TSNoRebuild, "ts-no-rebuild", false, "Reuse the existing TypeScript bundle instead of rebuilding it", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                tsFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", common.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(action.DefaultChartPackLogLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
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
