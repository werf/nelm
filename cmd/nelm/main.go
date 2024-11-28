package main

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/commands"
	"github.com/werf/nelm/pkg/log"
)

func main() {
	// Initialize the logger
	logger := log.NewLogboekLogger()

	var rootCmd = &cobra.Command{
		Use:   "nelm",
		Short: "Nelm is a Helm 3 replacement with enhanced features",
		Long:  `Nelm is designed to be a direct replacement for Helm 3, offering additional capabilities and improvements.`,
		// Silence Cobra's automatic error and usage messages
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Add subcommands
	rootCmd.AddCommand(commands.NewUninstallCommand())
	rootCmd.AddCommand(commands.NewRenderCommand())
	rootCmd.AddCommand(commands.NewDeployCommand())
	rootCmd.AddCommand(commands.NewPlanCommand())

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		// Log the error
		logger.Error(context.Background(), "Error: %v", err)

		// Check if the error message contains "unknown flag"
		if strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "unknown shorthand flag") {
			// Show help for the command that failed
			if cmd, _, err := rootCmd.Find(os.Args[1:]); err == nil {
				cmd.Help()
			}
		}

		os.Exit(1)
	}
}
