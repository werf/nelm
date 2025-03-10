package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretKeyRotateOptions struct {
	ChartDirPath      string
	NewKey            string
	OldKey            string
	SecretValuesPaths []string

	logLevel string
}

func (c *chartSecretKeyRotateOptions) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartSecretKeyRotateCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretKeyRotateOptions{}

	cmd := &cobra.Command{
		Use:   "rotate [options...] --old-key secret-key --new-key secret-key [chart-dir]",
		Short: "Reencrypt secret files with a new secret key.",
		Long:  "Decrypt with an old secret key, then encrypt with a new secret key chart files secret-values.yaml and secret/*.",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				cfg.ChartDirPath = args[0]
			}

			if err := action.SecretKeyRotate(ctx, action.SecretKeyRotateOptions{
				ChartDirPath:      cfg.ChartDirPath,
				LogLevel:          cfg.LogLevel(),
				NewKey:            cfg.NewKey,
				OldKey:            cfg.OldKey,
				SecretValuesPaths: cfg.SecretValuesPaths,
			}); err != nil {
				return fmt.Errorf("secret key rotate: %w", err)
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

		if err := flag.Add(cmd, &cfg.NewKey, "new-key", "", "New secret key", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.OldKey, "old-key", "", "Old secret key", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.SecretValuesPaths, "secret-values", []string{}, "Secret values files paths", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
			Type:                 flag.TypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
