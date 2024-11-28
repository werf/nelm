package commands

import (
	"context"
	"fmt"
	"github.com/werf/logboek"

	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/action"
)

func NewRenderCommand() *cobra.Command {
	var opts action.RenderOptions

	cmd := &cobra.Command{
		Use:     "render [release-name] [chart-dir]",
		Short:   "Render Helm charts to Kubernetes manifests",
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"template"},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ReleaseName = args[0]
			if len(args) > 1 {
				opts.ChartDirPath = args[1]
			} else {
				opts.ChartDirPath = ""
			}

			ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())
			if err := action.Render(ctx, opts); err != nil {
				return fmt.Errorf("render failed: %w", err)
			}
			return nil
		},
	}

	// Define flags
	cmd.Flags().BoolVar(&opts.ChartRepositoryInsecure, "plain-http", false, "use insecure HTTP connections for the chart download")
	cmd.Flags().BoolVar(&opts.ChartRepositorySkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS certificate verification when pulling images")
	cmd.Flags().BoolVar(&opts.ChartRepositorySkipUpdate, "skip-dependency-update", false, "Skip updating the chart repository index")
	cmd.Flags().BoolVar(&opts.DefaultSecretValuesDisable, "disable-default-secret-values", false, "Disable default secret values")
	cmd.Flags().BoolVar(&opts.DefaultValuesDisable, "disable-default-values", false, "Disable default values")
	cmd.Flags().StringToStringVarP(&opts.ExtraAnnotations, "annotations", "a", map[string]string{}, "Extra annotations to add to the rendered manifests")
	cmd.Flags().StringToStringVarP(&opts.ExtraLabels, "labels", "l", map[string]string{}, "Extra labels to add to the rendered manifests")
	cmd.Flags().StringToStringVar(&opts.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Extra runtime annotations to add to the rendered manifests")
	cmd.Flags().StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	cmd.Flags().StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	cmd.Flags().StringVar(&opts.KubeContext, "kube-context", "", "Kubernetes context to use")
	cmd.Flags().BoolVar(&opts.Local, "local", false, "Render locally without accessing the Kubernetes cluster")
	cmd.Flags().StringVar(&opts.LocalKubeVersion, "kube-version", "", "Local Kubernetes version")
	cmd.Flags().BoolVar(&opts.LogDebug, "debug", false, "Enable debug logging")
	cmd.Flags().IntVar(&opts.NetworkParallelism, "network-parallelism", 30, "Network parallelism")
	cmd.Flags().StringVar(&opts.RegistryCredentialsPath, "registry-credentials-path", "", "Registry credentials path")
	cmd.Flags().StringVar(&opts.ReleaseNamespace, "namespace", "", "Release namespace")
	cmd.Flags().StringVar(&opts.OutputFilePath, "output-path", "", "Output file path")
	cmd.Flags().BoolVar(&opts.OutputFileSave, "output", false, "Output file save")
	cmd.Flags().BoolVar(&opts.SecretKeyIgnore, "ignore-secret-key", false, "Secret key ignore")
	cmd.Flags().StringSliceVar(&opts.SecretValuesPaths, "secret-values", []string{}, "Secret values paths")
	cmd.Flags().BoolVar(&opts.ShowCRDs, "show-crds", false, "Show CRDs")
	cmd.Flags().StringSliceVar(&opts.ShowOnlyFiles, "show-only-files", []string{}, "Show only files")
	cmd.Flags().StringVar(&opts.TempDirPath, "temp-dir", "", "Temp dir path")
	cmd.Flags().StringSliceVar(&opts.ValuesFileSets, "set-file", []string{}, "Values file sets")
	cmd.Flags().StringSliceVar(&opts.ValuesFilesPaths, "values", []string{}, "Values files paths")
	cmd.Flags().StringSliceVar(&opts.ValuesSets, "set", []string{}, "Values sets")
	cmd.Flags().StringSliceVar(&opts.ValuesStringSets, "set-string", []string{}, "Values string sets")

	return cmd
}

