package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/v2/pkg/action"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	helmcmd "github.com/werf/nelm/v2/pkg/helm/pkg/cmd"
	"github.com/werf/nelm/v2/pkg/log"
)

func newChartPackCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	var (
		defaultPatchesDisable      bool
		defaultSecretValuesDisable bool
		valuesOpts                 common.ValuesOptions
	)

	opts := action.ChartPackOptions{}
	cmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "package")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Use = "pack [CHART_PATH] [...]"
	cmd.Short = "Pack a chart into an archive to distribute via a repository."
	cmd.Long = strings.ReplaceAll(cmd.Long, "helm package", "nelm chart pack")
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 30, chartCmdGroup)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helmcmd.Settings

		ctx = action.SetupLogging(cmd.Context(), lo.Ternary(helmSettings.Debug, log.DebugLevel, log.InfoLevel), action.SetupLoggingOptions{})
		cmd.SetContext(ctx)

		loader.NoChartLockWarning = ""

		if len(args) == 0 {
			return errors.New("need at least one argument, the path to the chart")
		}

		if err := setChartPackHelmFlagOptions(cmd, &opts); err != nil {
			return err
		}

		opts.RegistryCredentialsPath = helmSettings.RegistryConfig
		opts.RepositoryConfig = helmSettings.RepositoryConfig
		opts.RepositoryCache = helmSettings.RepositoryCache
		opts.ContentCache = helmSettings.ContentCache

		opts.RootSetJSON = valuesOpts.RootSetJSON
		opts.ValuesFiles = valuesOpts.ValuesFiles
		opts.ValuesSet = valuesOpts.ValuesSet
		opts.ValuesSetFile = valuesOpts.ValuesSetFile
		opts.ValuesSetJSON = valuesOpts.ValuesSetJSON
		opts.ValuesSetLiteral = valuesOpts.ValuesSetLiteral
		opts.ValuesSetString = valuesOpts.ValuesSetString

		for flagName, target := range map[string]struct {
			opt   **bool
			value bool
		}{
			"no-default-values":        {opt: &opts.DefaultValuesDisable, value: valuesOpts.DefaultValuesDisable},
			"no-default-secret-values": {opt: &opts.DefaultSecretValuesDisable, value: defaultSecretValuesDisable},
			"no-default-patches":       {opt: &opts.DefaultPatchesDisable, value: defaultPatchesDisable},
		} {
			if cmd.Flags().Changed(flagName) {
				*target.opt = lo.ToPtr(target.value)
			}
		}

		for _, chartPath := range args {
			if _, err := os.Stat(chartPath); err != nil {
				return fmt.Errorf("stat chart path: %w", err)
			}

			if _, err := action.ChartPack(ctx, chartPath, opts); err != nil {
				return fmt.Errorf("chart pack: %w", err)
			}
		}

		return nil
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := AddValuesFlags(cmd, &valuesOpts); err != nil {
			return fmt.Errorf("add values flags: %w", err)
		}

		if err := cli.AddFlag(cmd, &defaultSecretValuesDisable, "no-default-secret-values", false, "Ignore secret-values.yaml of the top-level chart", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &opts.SecretValuesFiles, "secret-values", []string{}, "Secret values files paths. Every value must be encrypted", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := AddPatchesFlags(cmd, &opts.PatchesFiles, &defaultPatchesDisable, AddPatchesFlagsOptions{}); err != nil {
			return fmt.Errorf("add patches flags: %w", err)
		}

		if err := cli.AddFlag(cmd, &opts.ResetValues, "reset-values", false, "Drop all publish-time inputs stored in .werf-data of the chart by previous packs (values, secret values, --set*, --set-root-json, patches, --no-default-* switches, release defaults, extra values, annotations, labels and images, including the annotations and labels moved from extra_annotations.json and extra_labels.json of a werf <= 3.12 bundle) before storing the ones of this pack", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &opts.Name, "name", "", "Set the chart name", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &opts.ChartRepoSkipUpdate, "no-update-chart-repos", false, "Don't update chart repositories index", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &opts.DenoBinaryPath, "deno-binary-path", "", "Path to the Deno binary to use instead of auto-downloading.", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                tsFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &opts.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, use the default system directory for temporary files", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &opts.TSNoRebuild, "ts-no-rebuild", false, "Reuse the existing TypeScript bundle instead of rebuilding it", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                tsFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}

// setChartPackHelmFlagOptions sets the options from the flags of Helm's package command.
func setChartPackHelmFlagOptions(cmd *cobra.Command, opts *action.ChartPackOptions) error {
	flags := cmd.Flags()

	for flagName, target := range map[string]*string{
		"version":         &opts.Version,
		"app-version":     &opts.AppVersion,
		"destination":     &opts.Destination,
		"key":             &opts.SignKey,
		"keyring":         &opts.SignKeyring,
		"passphrase-file": &opts.SignPassphraseFile,
		"username":        &opts.ChartRepoBasicAuthUsername,
		"password":        &opts.ChartRepoBasicAuthPassword,
		"cert-file":       &opts.ChartRepoCertPath,
		"key-file":        &opts.ChartRepoKeyPath,
		"ca-file":         &opts.ChartRepoCAPath,
	} {
		value, err := flags.GetString(flagName)
		if err != nil {
			return fmt.Errorf("get flag %q: %w", flagName, err)
		}

		*target = value
	}

	for flagName, target := range map[string]*bool{
		"sign":                     &opts.Sign,
		"dependency-update":        &opts.DependencyUpdate,
		"insecure-skip-tls-verify": &opts.ChartRepoSkipTLSVerify,
		"plain-http":               &opts.ChartRepoInsecure,
	} {
		value, err := flags.GetBool(flagName)
		if err != nil {
			return fmt.Errorf("get flag %q: %w", flagName, err)
		}

		*target = value
	}

	if opts.Sign && opts.SignKey == "" {
		return errors.New("--key is required for signing a package")
	}

	if opts.Sign && opts.SignKeyring == "" {
		return errors.New("--keyring is required for signing a package")
	}

	return nil
}
