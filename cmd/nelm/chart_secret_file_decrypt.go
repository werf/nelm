package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretFileDecryptOptions struct {
	OutputFilePath string
	SecretKey      string
	File           string

	logLevel string
}

func (c *chartSecretFileDecryptOptions) OutputFileSave() bool {
	return c.OutputFilePath != ""
}

func (c *chartSecretFileDecryptOptions) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartSecretFileDecryptCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretFileDecryptOptions{}

	cmd := &cobra.Command{
		Use:   "decrypt [options...] --secret-key secret-key file",
		Short: "Decrypt file and print result to stdout.",
		Long:  "Decrypt file and print result to stdout.",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.File = args[0]

			if err := action.SecretFileDecrypt(ctx, cfg.File, action.SecretFileDecryptOptions{
				LogLevel:       cfg.LogLevel(),
				OutputFilePath: cfg.OutputFilePath,
				OutputFileSave: cfg.OutputFileSave(),
				SecretKey:      cfg.SecretKey,
			}); err != nil {
				return fmt.Errorf("secret file decrypt: %w", err)
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

		return nil
	}

	return cmd
}
