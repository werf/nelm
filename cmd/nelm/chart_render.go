package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/action"
)

func NewChartRenderCommand(ctx context.Context) *cobra.Command {
	var opts action.RenderOptions

	cmd := &cobra.Command{
		Use:   "render [-n namespace] [-r release] [chart-dir]",
		Short: "Render a Helm Chart.",
		Long:  "Render a Helm Chart.",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.ChartDirPath = args[0]
			}

			if err := action.Render(ctx, opts); err != nil {
				return fmt.Errorf("render: %w", err)
			}

			return nil
		},
	}

	f := cmd.Flags()

	f.BoolVar(&opts.ChartRepositoryInsecure, "plain-http", false, "use insecure HTTP connections for the chart download")
	f.BoolVar(&opts.ChartRepositorySkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS certificate verification when pulling images")
	f.BoolVar(&opts.ChartRepositorySkipUpdate, "skip-dependency-update", false, "Skip updating the chart repository index")
	f.BoolVar(&opts.DefaultSecretValuesDisable, "disable-default-secret-values", false, "Disable default secret values")
	f.BoolVar(&opts.DefaultValuesDisable, "disable-default-values", false, "Disable default values")
	f.StringToStringVarP(&opts.ExtraAnnotations, "annotations", "a", map[string]string{}, "Extra annotations to add to the rendered manifests")
	f.StringToStringVarP(&opts.ExtraLabels, "labels", "l", map[string]string{}, "Extra labels to add to the rendered manifests")
	f.StringToStringVar(&opts.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Extra runtime annotations to add to the rendered manifests")
	f.StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	f.StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	f.StringVar(&opts.KubeContext, "kube-context", "", "Kubernetes context to use")
	f.BoolVar(&opts.Local, "local", false, "Render locally without accessing the Kubernetes cluster")
	f.StringVar(&opts.LocalKubeVersion, "kube-version", "", "Local Kubernetes version")
	f.BoolVar(&opts.LogDebug, "debug", false, "Enable debug logging")
	f.IntVar(&opts.NetworkParallelism, "network-parallelism", 30, "Network parallelism")
	f.StringVar(&opts.RegistryCredentialsPath, "registry-credentials-path", "", "Registry credentials path")
	f.StringVarP(&opts.ReleaseNamespace, "namespace", "n", "namespace-stub", "Release namespace")
	f.StringVar(&opts.OutputFilePath, "output-path", "", "Output file path")
	f.BoolVar(&opts.OutputFileSave, "output", false, "Output file save")
	f.BoolVar(&opts.SecretKeyIgnore, "ignore-secret-key", false, "Secret key ignore")
	f.StringSliceVar(&opts.SecretValuesPaths, "secret-values", []string{}, "Secret values paths")
	f.BoolVar(&opts.ShowCRDs, "show-crds", false, "Show CRDs")
	f.StringSliceVar(&opts.ShowOnlyFiles, "show-only-files", []string{}, "Show only files")
	f.StringVar(&opts.TempDirPath, "temp-dir", "", "Temp dir path")
	f.StringSliceVar(&opts.ValuesFileSets, "set-file", []string{}, "Values file sets")
	f.StringSliceVar(&opts.ValuesFilesPaths, "values", []string{}, "Values files paths")
	f.StringSliceVar(&opts.ValuesSets, "set", []string{}, "Values sets")
	f.StringSliceVar(&opts.ValuesStringSets, "set-string", []string{}, "Values string sets")
	f.StringVarP(&opts.ReleaseName, "release", "r", "release-stub", "Release name")

	return cmd
}
