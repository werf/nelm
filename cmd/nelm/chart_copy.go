package main

import (
	"cmp"
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/v2/pkg/action"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/log"
)

type chartCopyConfig struct {
	action.ChartCopyOptions

	ArchiveMaxExpandedSize string
	LogColorMode           string
	LogLevel               string
	LogWidth               int
}

func newChartCopyCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartCopyConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"copy FROM TO",
		"Copy a chart with its images.",
		"Copy a chart with every image listed in its helm.sh/images annotation. FROM and TO are either an OCI repository, oci://REPO[:TAG] (docker://REPO[:TAG] or REPO[:TAG] also work), or an archive, archive:PATH or a path ending in .tar, .tar.gz or .tgz. A source repository without a tag means the \"latest\" tag. An archive is written as an OCI image layout; werf bundle archives are also read. Copied to a repository, the chart is renamed to the repository basename and its images are copied into the repository by digest, keeping their tags. With TAG the chart version is TAG if it is a semver version (without the \"v\" prefix), or 0.0.0-<unix time>-<TAG slug> otherwise, and the chart is pushed under the version and TAG, with a \"+\" in either pushed as \"_\"; without TAG the chart keeps its version and is pushed under it and the source tag, if any (a werf bundle archive has none). Images of chart dependencies are copied too, non-distributable layers included; referrers of the images, like signatures and SBOMs, are not copied. Loopback registries, like localhost:5000, are reached over plain HTTP.",
		35, // priority for ordering in help
		chartCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.ExactArgs(2),
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = action.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), log.InfoLevel), action.SetupLoggingOptions{
				ColorMode: cfg.LogColorMode,
				LogWidth:  cfg.LogWidth,
			})

			maxExpandedSize, err := resource.ParseQuantity(cfg.ArchiveMaxExpandedSize)
			if err != nil {
				return fmt.Errorf("parse --archive-max-expanded-size: %w", err)
			}

			if maxExpandedSize.Value() <= 0 {
				return fmt.Errorf("--archive-max-expanded-size must be positive, got %q", cfg.ArchiveMaxExpandedSize)
			}

			cfg.ChartCopyOptions.ArchiveMaxExpandedSize = maxExpandedSize.Value()

			if err := action.ChartCopy(ctx, args[0], args[1], cfg.ChartCopyOptions); err != nil {
				return fmt.Errorf("chart copy: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := AddChartRepoConnectionFlags(cmd, &cfg.ChartRepoConnectionOptions); err != nil {
			return fmt.Errorf("add chart repo connection flags: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.RegistryCredentialsPath, "oci-chart-repos-creds", "", "Credentials to access OCI chart repositories", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ArchiveMaxExpandedSize, "archive-max-expanded-size", resource.NewQuantity(action.DefaultChartCopyArchiveMaxExpandedSize, resource.BinarySI).String(), "Maximum size the source archive may decompress to, nested image tarballs included, and, separately, the maximum size extracted from its tars, e.g. 64Gi. Temporary files may take up to about twice as much", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.Sign, "sign", false, "Sign the copy if it changes the chart. Without it, the provenance of a changed chart is dropped", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SignKey, "sign-key", "", "Name of the key to sign with", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SignKeyring, "sign-keyring", "", "Path to the keyring with the signing key", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SignPassphraseFile, "sign-passphrase-file", "", "Path to a file with the passphrase of the signing key, \"-\" for stdin", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", common.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(log.InfoLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
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

		if err := cli.AddFlag(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files. Reading an archive needs space for about the decompressed archive and twice its images", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
