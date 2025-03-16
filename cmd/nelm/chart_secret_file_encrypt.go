package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretFileEncryptOptions struct {
	File           string
	OutputFilePath string
	SecretKey      string
	TempDirPath    string

	logColorMode string
	logLevel     string
}

func (c *chartSecretFileEncryptOptions) OutputFileSave() bool {
	return c.OutputFilePath != ""
}

func (c *chartSecretFileEncryptOptions) LogColorMode() action.LogColorMode {
	return action.LogColorMode(c.logColorMode)
}

func (c *chartSecretFileEncryptOptions) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartSecretFileEncryptCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretFileEncryptOptions{}

	cmd := &cobra.Command{
		Use:   "encrypt [options...] --secret-key secret-key file",
		Short: "Encrypt file and print result to stdout.",
		Long:  "Encrypt file and print result to stdout.",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.File = args[0]

			if err := action.SecretFileEncrypt(ctx, cfg.File, action.SecretFileEncryptOptions{
				LogColorMode:   cfg.LogColorMode(),
				LogLevel:       cfg.LogLevel(),
				OutputFilePath: cfg.OutputFilePath,
				OutputFileSave: cfg.OutputFileSave(),
				SecretKey:      cfg.SecretKey,
				TempDirPath:    cfg.TempDirPath,
			}); err != nil {
				return fmt.Errorf("secret file encrypt: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := flag.Add(cmd, &cfg.logColorMode, "color-mode", string(action.DefaultLogColorMode), "Color mode for logs. "+allowedLogColorModesHelp(), flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(action.DefaultSecretFileEncryptLogLevel), "Set log level. "+allowedLogLevelsHelp(), flag.AddOptions{
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

		if err := flag.Add(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 flag.TypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
