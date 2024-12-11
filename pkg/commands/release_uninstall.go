package commands

import (
	"context"
	"fmt"
	"time"
	"github.com/werf/logboek"

	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/action"
)


func NewReleaseUninstallCommand() *cobra.Command {
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

	f := cmd.Flags()
	// Define flags
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

	return cmd
}
