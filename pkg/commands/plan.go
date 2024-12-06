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
	
	f := cmd.Flags()
	// Define flags
	f.BoolVar(&opts.ChartRepositoryInsecure, "plain-http", false, "use insecure HTTP connections for the chart download")
	f.BoolVar(&opts.ChartRepositorySkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS verification for chart repository")
	f.BoolVar(&opts.ChartRepositorySkipUpdate, "skip-dependency-update", false, "Skip update of the chart repository")
	f.BoolVar(&opts.DefaultSecretValuesDisable, "disable-default-secret-values", false, "Disable default secret values")
	f.BoolVar(&opts.DefaultValuesDisable, "disable-default-values", false, "Disable default values")
	f.BoolVar(&opts.ErrorIfChangesPlanned, "exit-on-changes", false, "Exit with error if changes are planned")
	f.StringToStringVarP(&opts.ExtraAnnotations, "annotations", "a", map[string]string{}, "Extra annotations to add to the rendered manifests")
	f.StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	f.StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	f.StringVar(&opts.KubeContext, "kube-context", "", "Kube context to use")
	f.BoolVar(&opts.LogDebug, "debug", false, "Enable debug logging")
	f.IntVar(&opts.NetworkParallelism, "network-parallelism", 30, "Network parallelism")
	f.StringVar(&opts.RegistryCredentialsPath, "registry-credentials-path", "", "Path to the registry credentials")
	f.StringVar(&opts.ReleaseNamespace, "namespace", "default", "Namespace for the release")
	f.BoolVar(&opts.SecretKeyIgnore, "ignore-secret-key", false, "Ignore secret keys")
	f.StringSliceVar(&opts.SecretValuesPaths, "secret-values", []string{}, "Paths to secret values files")
	f.StringVar(&opts.TempDirPath, "temp-dir", "", "Path to the temporary directory")
	f.StringSliceVar(&opts.ValuesFileSets, "set-file", []string{}, "Values file sets")
	f.StringSliceVarP(&opts.ValuesFilesPaths, "values", "f", []string{}, "Paths to values files\n(can be set multiple times)")
	f.StringSliceVar(&opts.ValuesSets, "set", []string{}, "Values sets")
	f.StringSliceVar(&opts.ValuesStringSets, "set-string", []string{}, "Values string sets")

	return cmd
}
