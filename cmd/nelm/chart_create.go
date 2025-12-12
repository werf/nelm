package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/internal/tschart"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

type chartCreateConfig struct {
	OnlyTS bool
}

func newChartCreateCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartCreateConfig{}

	cmd := lo.Must(lo.Find(helmRootCmd.Commands(), func(c *cobra.Command) bool {
		return strings.HasPrefix(c.Use, "create")
	}))

	cmd.LocalFlags().AddFlagSet(cmd.InheritedFlags())
	cmd.Use = "create NAME [PATH]"
	cmd.Short = "Create a new chart with the given name."
	cmd.Long = strings.ReplaceAll(cmd.Long, "helm create", "nelm chart create")
	cmd.Aliases = []string{}
	cli.SetSubCommandAnnotations(cmd, 10, chartCmdGroup)

	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		helmSettings := helm_v3.Settings
		ctx = log.SetupLogging(ctx, lo.Ternary(helmSettings.Debug, log.DebugLevel, log.InfoLevel), log.SetupLoggingOptions{})

		// Handle --only-ts mode
		if cfg.OnlyTS {
			if !featgate.FeatGateTypescript.Enabled() {
				return fmt.Errorf("--only-ts requires NELM_FEAT_TYPESCRIPT=true")
			}

			if len(args) == 0 {
				return fmt.Errorf("chart name is required")
			}

			chartName := args[0]
			chartPath := chartName
			if len(args) > 1 {
				chartPath = filepath.Join(args[1], args[0])
			}

			// Create directory if it doesn't exist
			if err := os.MkdirAll(chartPath, 0755); err != nil {
				return fmt.Errorf("create directory %s: %w", chartPath, err)
			}

			// Check if Chart.yaml exists
			chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
			chartExists := false
			if _, err := os.Stat(chartYamlPath); err == nil {
				chartExists = true
			}

			// If no existing chart, create chart structure (without templates/)
			if !chartExists {
				if err := tschart.CreateTSOnlyChartStructure(ctx, chartPath, chartName); err != nil {
					return fmt.Errorf("create chart structure: %w", err)
				}
				log.Default.Info(ctx, "Created chart structure in %s", chartPath)
			}

			if err := tschart.CreateTSBoilerplate(ctx, chartPath, chartName); err != nil {
				return fmt.Errorf("create TypeScript boilerplate: %w", err)
			}

			helmignorePath := filepath.Join(chartPath, ".helmignore")
			if _, err := os.Stat(helmignorePath); err == nil {
				if err := tschart.AppendToHelmignore(chartPath); err != nil {
					return fmt.Errorf("update .helmignore: %w", err)
				}
			}

			log.Default.Info(ctx, "Created TypeScript chart in %s", chartPath)
			return nil
		}

		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		if featgate.FeatGateTypescript.Enabled() && len(args) > 0 {
			chartPath := args[0]
			chartName := filepath.Base(chartPath)

			if err := tschart.CreateTSBoilerplate(ctx, chartPath, chartName); err != nil {
				return fmt.Errorf("create TypeScript boilerplate: %w", err)
			}

			if err := tschart.AppendToHelmignore(chartPath); err != nil {
				return fmt.Errorf("update .helmignore: %w", err)
			}

			log.Default.Info(ctx, "Added TypeScript chart support to %s", chartPath)
		}

		return nil
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.OnlyTS, "only-ts", false, "Create TypeScript-only chart without templates/ directory (requires NELM_FEAT_TYPESCRIPT=true)", cli.AddFlagOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}
		return nil
	}

	return cmd
}
