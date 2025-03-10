package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretValuesFileEditOptions struct {
	OutputFilePath string
	SecretKey      string
	TempDirPath    string
	ValuesFile     string

	logLevel string
}

func (c *chartSecretValuesFileEditOptions) OutputFileSave() bool {
	return c.OutputFilePath != ""
}

func (c *chartSecretValuesFileEditOptions) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartSecretValuesFileEditCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretValuesFileEditOptions{}

	cmd := &cobra.Command{
		Use:   "edit [options...] --secret-key secret-key values-file",
		Short: "Interactively edit encrypted values file.",
		Long:  "Interactively edit encrypted values file.",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ValuesFile = args[0]

			if err := action.SecretValuesFileEdit(ctx, cfg.ValuesFile, action.SecretValuesFileEditOptions{
				LogLevel:    cfg.LogLevel(),
				TempDirPath: cfg.TempDirPath,
				SecretKey:   cfg.SecretKey,
			}); err != nil {
				return fmt.Errorf("secret values file edit: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		// FIXME(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(log.InfoLevel), "Set log level", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.SecretKey, "secret-key", "", "Secret key", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", flag.AddOptions{
			Group: miscFlagGroup,
			Type:  flag.TypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
