package commands

import (
	"context"
	"fmt"
	"github.com/werf/logboek"

	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/action"
)

func NewPlanCommand() *cobra.Command {
	var opts action.PlanOptions

	cmd := &cobra.Command{
		Use:     "plan [release-name] [chart-dir]",
		Short:   "Plan a Helm chart",
		Long:    "Plan a Helm chart with the specified release name.",
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"upgrade", "install"},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ReleaseName = args[0]
			if len(args) > 1 {
				opts.ChartDirPath = args[1]
			} else {
				opts.ChartDirPath = ""
			}

			ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())
			if err := action.Plan(ctx, opts); err != nil {
				return fmt.Errorf("plan failed: %w", err)
			}
			return nil
		},
	}

	// Define flags
	cmd.Flags().BoolVar(&opts.ChartRepositoryInsecure, "plain-http", false, "use insecure HTTP connections for the chart download")
	cmd.Flags().BoolVar(&opts.ChartRepositorySkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS verification for chart repository")
	cmd.Flags().BoolVar(&opts.ChartRepositorySkipUpdate, "skip-dependency-update", false, "Skip update of the chart repository")
	cmd.Flags().BoolVar(&opts.DefaultSecretValuesDisable, "disable-default-secret-values", false, "Disable default secret values")
	cmd.Flags().BoolVar(&opts.DefaultValuesDisable, "disable-default-values", false, "Disable default values")
	cmd.Flags().BoolVar(&opts.ErrorIfChangesPlanned, "exit-on-changes", false, "Exit with error if changes are planned")
	cmd.Flags().StringToStringVarP(&opts.ExtraAnnotations, "annotations", "a", map[string]string{}, "Extra annotations to add to the rendered manifests")
	cmd.Flags().StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	cmd.Flags().StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	cmd.Flags().StringVar(&opts.KubeContext, "kube-context", "", "Kube context to use")
	cmd.Flags().BoolVar(&opts.LogDebug, "debug", false, "Enable debug logging")
	cmd.Flags().IntVar(&opts.NetworkParallelism, "network-parallelism", 30, "Network parallelism")
	cmd.Flags().StringVar(&opts.RegistryCredentialsPath, "registry-credentials-path", "", "Path to the registry credentials")
	cmd.Flags().StringVar(&opts.ReleaseNamespace, "namespace", "default", "Namespace for the release")
	cmd.Flags().BoolVar(&opts.SecretKeyIgnore, "ignore-secret-key", false, "Ignore secret keys")
	cmd.Flags().StringSliceVar(&opts.SecretValuesPaths, "secret-values", []string{}, "Paths to secret values files")
	cmd.Flags().StringVar(&opts.TempDirPath, "temp-dir", "", "Path to the temporary directory")
	cmd.Flags().StringSliceVar(&opts.ValuesFileSets, "set-file", []string{}, "Values file sets")
	cmd.Flags().StringSliceVarP(&opts.ValuesFilesPaths, "values", "f", []string{}, "Paths to values files\n(can be set multiple times)")
	cmd.Flags().StringSliceVar(&opts.ValuesSets, "set", []string{}, "Values sets")
	cmd.Flags().StringSliceVar(&opts.ValuesStringSets, "set-string", []string{}, "Values string sets")
	return cmd
}
