package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/flag"
)

type releaseUninstallConfig struct {
	DeleteHooks                bool
	DeleteReleaseNamespace     bool
	KubeAPIServerName          string
	KubeBurstLimit             int
	KubeCAPath                 string
	KubeConfigBase64           string
	KubeConfigPaths            []string
	KubeContext                string
	KubeQPSLimit               int
	KubeSkipTLSVerify          bool
	KubeTLSServerName          string
	KubeToken                  string
	LogDebug                   bool
	ProgressTablePrintInterval time.Duration
	ReleaseHistoryLimit        int
	ReleaseName                string
	ReleaseNamespace           string
	ReleaseStorageDriver       action.ReleaseStorageDriver
	TempDirPath                string
}

func newReleaseUninstallCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releaseUninstallConfig{}

	cmd := &cobra.Command{
		Use:                   "uninstall [options...] -n namespace release",
		Short:                 "Uninstall a Helm Release from Kubernetes.",
		Long:                  "Uninstall a Helm Release from Kubernetes.",
		Args:                  cobra.ExactArgs(1),
		ValidArgsFunction:     cobra.NoFileCompletions,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ReleaseName = args[0]

			if err := action.Uninstall(ctx, action.UninstallOptions{
				DeleteHooks:                cfg.DeleteHooks,
				DeleteReleaseNamespace:     cfg.DeleteReleaseNamespace,
				KubeAPIServerName:          cfg.KubeAPIServerName,
				KubeBurstLimit:             cfg.KubeBurstLimit,
				KubeCAPath:                 cfg.KubeCAPath,
				KubeConfigBase64:           cfg.KubeConfigBase64,
				KubeConfigPaths:            cfg.KubeConfigPaths,
				KubeContext:                cfg.KubeContext,
				KubeQPSLimit:               cfg.KubeQPSLimit,
				KubeSkipTLSVerify:          cfg.KubeSkipTLSVerify,
				KubeTLSServerName:          cfg.KubeTLSServerName,
				KubeToken:                  cfg.KubeToken,
				LogDebug:                   cfg.LogDebug,
				ProgressTablePrintInterval: cfg.ProgressTablePrintInterval,
				ReleaseHistoryLimit:        cfg.ReleaseHistoryLimit,
				ReleaseName:                cfg.ReleaseName,
				ReleaseNamespace:           cfg.ReleaseNamespace,
				ReleaseStorageDriver:       cfg.ReleaseStorageDriver,
				TempDirPath:                cfg.TempDirPath,
			}); err != nil {
				return fmt.Errorf("uninstall: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := flag.Add(
			cmd,
			&cfg.ReleaseNamespace,
			"namespace",
			"",
			"Namespace of the release",
			flag.AddOptions{
				ShortName: "n",
				Required:  true,
			},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.DeleteHooks,
			"delete-hooks",
			false,
			"Delete hooks",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.DeleteReleaseNamespace,
			"delete-namespace",
			false,
			"Delete namespace of the release",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.KubeConfigBase64,
			"kubeconfig-base64",
			"",
			"Base64 encoded kube config",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.KubeConfigPaths,
			"kubeconfig",
			[]string{},
			"Paths to kube config files\n(can be set multiple times)",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.KubeContext,
			"kube-context",
			"",
			"Kubernetes context to use",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.LogDebug,
			"debug",
			false,
			"enable verbose output",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ProgressTablePrintInterval,
			"kubedog-interval",
			5*time.Second,
			"Progress print interval",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ReleaseHistoryLimit,
			"keep-history-limit",
			10,
			"Release history limit (0 to remove all history)",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.TempDirPath,
			"temp-dir",
			"",
			"Path to the temporary directory",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
