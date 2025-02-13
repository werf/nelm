package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/action"
)

func NewReleaseUninstallCommand(ctx context.Context) *cobra.Command {
	var opts action.UninstallOptions

	cmd := &cobra.Command{
		Use:                   "uninstall [options...] -n namespace release",
		Short:                 "Uninstall a Helm Release from Kubernetes.",
		Long:                  "Uninstall a Helm Release from Kubernetes.",
		Args:                  cobra.ExactArgs(1),
		ValidArgsFunction:     cobra.NoFileCompletions,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ReleaseName = args[0]

			if err := action.Uninstall(ctx, opts); err != nil {
				return fmt.Errorf("uninstall: %w", err)
			}

			return nil
		},
	}

	f := cmd.Flags()

	f.StringVarP(&opts.ReleaseNamespace, "namespace", "n", "default", "Namespace of the release")
	f.BoolVar(&opts.DeleteHooks, "delete-hooks", false, "Delete hooks")
	f.BoolVar(&opts.DeleteReleaseNamespace, "delete-namespace", false, "Delete namespace of the release")
	f.StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	f.StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	f.StringVar(&opts.KubeContext, "kube-context", "", "Kubernetes context to use")
	f.BoolVar(&opts.LogDebug, "debug", false, "enable verbose output")
	f.DurationVar(&opts.ProgressTablePrintInterval, "kubedog-interval", 5*time.Second, "Progress print interval")
	f.IntVar(&opts.ReleaseHistoryLimit, "keep-history-limit", 10, "Release history limit (0 to remove all history)")
	f.StringVar(&opts.TempDirPath, "temp-dir", "", "Path to the temporary directory")

	cobra.MarkFlagRequired(f, "namespace")

	return cmd
}
