package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretValuesFileDecryptOptions struct {
	OutputFilePath string
	SecretKey      string
	TempDirPath    string
	ValuesFile     string

	logColorMode string
	logLevel     string
}

func (c *chartSecretValuesFileDecryptOptions) OutputFileSave() bool {
	return c.OutputFilePath != ""
}

func (c *chartSecretValuesFileDecryptOptions) LogColorMode() action.LogColorMode {
	return action.LogColorMode(c.logColorMode)
}

func (c *chartSecretValuesFileDecryptOptions) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartSecretValuesFileDecryptCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretValuesFileDecryptOptions{}

	cmd := &cobra.Command{
		Use:   "decrypt [options...] --secret-key secret-key values-file",
		Short: "Decrypt values file and print result to stdout.",
		Long:  "Decrypt values file and print result to stdout.",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ValuesFile = args[0]

			if err := action.SecretValuesFileDecrypt(ctx, cfg.ValuesFile, action.SecretValuesFileDecryptOptions{
				LogColorMode:   cfg.LogColorMode(),
				LogLevel:       cfg.LogLevel(),
				OutputFilePath: cfg.OutputFilePath,
				OutputFileSave: cfg.OutputFileSave(),
				SecretKey:      cfg.SecretKey,
				TempDirPath:    cfg.TempDirPath,
			}); err != nil {
				return fmt.Errorf("secret values file decrypt: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		// FIXME(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.logColorMode, "color-mode", string(action.DefaultLogColorMode), "Color mode for logs", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// FIXME(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(action.DefaultSecretValuesFileDecryptLogLevel), "Set log level", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.OutputFilePath, "save-output-to", "", "Save decrypted output to a file", flag.AddOptions{
			Type:  flag.TypeFile,
			Group: mainFlagGroup,
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
