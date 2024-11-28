package commands

import (
	"context"
	"fmt"
	"time"
	"github.com/werf/logboek"

	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/action"
)


func NewUninstallCommand() *cobra.Command {
	var opts action.UninstallOptions

	cmd := &cobra.Command{
		Use:   "uninstall [release-name]",
		Short: "Uninstall a Helm release",
		Long:  "Uninstall a Helm release with the specified release name.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ReleaseName = args[0]
			ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

			if err := action.Uninstall(ctx, opts); err != nil {
				return fmt.Errorf("uninstall failed: %w", err)
			}
			return nil
		},
	}

	// Define flags
	cmd.Flags().StringVarP(&opts.ReleaseNamespace, "namespace", "n", "default", "Namespace of the release")
	cmd.Flags().BoolVar(&opts.DeleteHooks, "delete-hooks", false, "Delete hooks")
	cmd.Flags().BoolVar(&opts.DeleteReleaseNamespace, "delete-namespace", false, "Delete namespace of the release")
	cmd.Flags().StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	cmd.Flags().StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	cmd.Flags().StringVar(&opts.KubeContext, "kube-context", "", "Kubernetes context to use")
	cmd.Flags().BoolVar(&opts.LogDebug, "debug", false, "enable verbose output")
	cmd.Flags().DurationVar(&opts.ProgressTablePrintInterval, "kubedog-interval", 5*time.Second, "Progress print interval")
	cmd.Flags().IntVar(&opts.ReleaseHistoryLimit, "keep-history-limit", 10, "Release history limit (0 to remove all history)")
	cmd.Flags().StringVar(&opts.TempDirPath, "temp-dir", "", "Path to the temporary directory")

	return cmd
}
