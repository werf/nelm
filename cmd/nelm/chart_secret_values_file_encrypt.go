package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretValuesFileEncryptOptions struct {
	OutputFilePath string
	SecretKey      string
	ValuesFile     string

	logLevel string
}

func (c *chartSecretValuesFileEncryptOptions) OutputFileSave() bool {
	return c.OutputFilePath != ""
}

func (c *chartSecretValuesFileEncryptOptions) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartSecretValuesFileEncryptCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretValuesFileEncryptOptions{}

	cmd := &cobra.Command{
		Use:   "encrypt [options...] --secret-key secret-key values-file",
		Short: "Encrypt values file and print result to stdout.",
		Long:  "Encrypt values file and print result to stdout.",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ValuesFile = args[0]

			if err := action.SecretValuesFileEncrypt(ctx, cfg.ValuesFile, action.SecretValuesFileEncryptOptions{
				LogLevel:       cfg.LogLevel(),
				OutputFilePath: cfg.OutputFilePath,
				OutputFileSave: cfg.OutputFileSave(),
				SecretKey:      cfg.SecretKey,
			}); err != nil {
				return fmt.Errorf("secret values file encrypt: %w", err)
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

		if err := flag.Add(cmd, &cfg.OutputFilePath, "save-output-to", "", "Save encrypted output to a file", flag.AddOptions{
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

		return nil
	}

	return cmd
}
