package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/flag"
)

type chartRenderConfig struct {
	ChartAppVersion              string
	ChartDirPath                 string
	ChartRepositoryInsecure      bool
	ChartRepositorySkipTLSVerify bool
	ChartRepositorySkipUpdate    bool
	DefaultChartAPIVersion       string
	DefaultChartName             string
	DefaultChartVersion          string
	DefaultSecretValuesDisable   bool
	DefaultValuesDisable         bool
	ExtraAnnotations             map[string]string
	ExtraLabels                  map[string]string
	ExtraRuntimeAnnotations      map[string]string
	KubeAPIServerName            string
	KubeBurstLimit               int
	KubeCAPath                   string
	KubeConfigBase64             string
	KubeConfigPaths              []string
	KubeContext                  string
	KubeQPSLimit                 int
	KubeSkipTLSVerify            bool
	KubeTLSServerName            string
	KubeToken                    string
	Local                        bool
	LocalKubeVersion             string
	LogDebug                     bool
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	OutputFilePath               string
	OutputFileSave               bool
	RegistryCredentialsPath      string
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         action.ReleaseStorageDriver
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SecretWorkDir                string
	ShowCRDs                     bool
	ShowOnlyFiles                []string
	TempDirPath                  string
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
}

func newChartRenderCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartRenderConfig{}

	cmd := &cobra.Command{
		Use:   "render [options...] [chart-dir]",
		Short: "Render a Helm Chart.",
		Long:  "Render a Helm Chart.",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				cfg.ChartDirPath = args[0]
			}

			if err := action.Render(ctx, action.RenderOptions{
				ChartAppVersion:              cfg.ChartAppVersion,
				ChartDirPath:                 cfg.ChartDirPath,
				ChartRepositoryInsecure:      cfg.ChartRepositoryInsecure,
				ChartRepositorySkipTLSVerify: cfg.ChartRepositorySkipTLSVerify,
				ChartRepositorySkipUpdate:    cfg.ChartRepositorySkipUpdate,
				DefaultChartAPIVersion:       cfg.DefaultChartAPIVersion,
				DefaultChartName:             cfg.DefaultChartName,
				DefaultChartVersion:          cfg.DefaultChartVersion,
				DefaultSecretValuesDisable:   cfg.DefaultSecretValuesDisable,
				DefaultValuesDisable:         cfg.DefaultValuesDisable,
				ExtraAnnotations:             cfg.ExtraAnnotations,
				ExtraLabels:                  cfg.ExtraLabels,
				ExtraRuntimeAnnotations:      cfg.ExtraRuntimeAnnotations,
				KubeAPIServerName:            cfg.KubeAPIServerName,
				KubeBurstLimit:               cfg.KubeBurstLimit,
				KubeCAPath:                   cfg.KubeCAPath,
				KubeConfigBase64:             cfg.KubeConfigBase64,
				KubeConfigPaths:              cfg.KubeConfigPaths,
				KubeContext:                  cfg.KubeContext,
				KubeQPSLimit:                 cfg.KubeQPSLimit,
				KubeSkipTLSVerify:            cfg.KubeSkipTLSVerify,
				KubeTLSServerName:            cfg.KubeTLSServerName,
				KubeToken:                    cfg.KubeToken,
				Local:                        cfg.Local,
				LocalKubeVersion:             cfg.LocalKubeVersion,
				LogDebug:                     cfg.LogDebug,
				LogRegistryStreamOut:         cfg.LogRegistryStreamOut,
				NetworkParallelism:           cfg.NetworkParallelism,
				OutputFilePath:               cfg.OutputFilePath,
				OutputFileSave:               cfg.OutputFileSave,
				RegistryCredentialsPath:      cfg.RegistryCredentialsPath,
				ReleaseName:                  cfg.ReleaseName,
				ReleaseNamespace:             cfg.ReleaseNamespace,
				ReleaseStorageDriver:         cfg.ReleaseStorageDriver,
				SecretKeyIgnore:              cfg.SecretKeyIgnore,
				SecretValuesPaths:            cfg.SecretValuesPaths,
				SecretWorkDir:                cfg.SecretWorkDir,
				ShowCRDs:                     cfg.ShowCRDs,
				ShowOnlyFiles:                cfg.ShowOnlyFiles,
				TempDirPath:                  cfg.TempDirPath,
				ValuesFileSets:               cfg.ValuesFileSets,
				ValuesFilesPaths:             cfg.ValuesFilesPaths,
				ValuesSets:                   cfg.ValuesSets,
				ValuesStringSets:             cfg.ValuesStringSets,
			}); err != nil {
				return fmt.Errorf("render: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := flag.Add(
			cmd,
			&cfg.ChartRepositoryInsecure,
			"plain-http",
			false,
			"use insecure HTTP connections for the chart download",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ChartRepositorySkipTLSVerify,
			"insecure-skip-tls-verify",
			false,
			"Skip TLS certificate verification when pulling images",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ChartRepositorySkipUpdate,
			"skip-dependency-update",
			false,
			"Skip updating the chart repository index",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.DefaultSecretValuesDisable,
			"disable-default-secret-values",
			false,
			"Disable default secret values",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.DefaultValuesDisable,
			"disable-default-values",
			false,
			"Disable default values",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ExtraAnnotations,
			"annotations",
			map[string]string{},
			"Extra annotations to add to the rendered manifests",
			flag.AddOptions{
				ShortName: "a",
			},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ExtraLabels,
			"labels",
			map[string]string{},
			"Extra labels to add to the rendered manifests",
			flag.AddOptions{
				ShortName: "l",
			},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ExtraRuntimeAnnotations,
			"runtime-annotations",
			map[string]string{},
			"Extra runtime annotations to add to the rendered manifests",
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
			&cfg.Local,
			"local",
			false,
			"Render locally without accessing the Kubernetes cluster",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.LocalKubeVersion,
			"kube-version",
			"",
			"Local Kubernetes version",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.LogDebug,
			"debug",
			false,
			"Enable debug logging",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.NetworkParallelism,
			"network-parallelism",
			30,
			"Network parallelism",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.RegistryCredentialsPath,
			"registry-credentials-path",
			"",
			"Registry credentials path",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ReleaseNamespace,
			"namespace",
			"namespace-stub",
			"Release namespace",
			flag.AddOptions{
				ShortName: "n",
			},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.OutputFilePath,
			"output-path",
			"",
			"Output file path",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.OutputFileSave,
			"output",
			false,
			"Output file save",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.SecretKeyIgnore,
			"ignore-secret-key",
			false,
			"Secret key ignore",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.SecretValuesPaths,
			"secret-values",
			[]string{},
			"Secret values paths",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ShowCRDs,
			"show-crds",
			false,
			"Show CRDs",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ShowOnlyFiles,
			"show-only-files",
			[]string{},
			"Show only files",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.TempDirPath,
			"temp-dir",
			"",
			"Temp dir path",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ValuesFileSets,
			"set-file",
			[]string{},
			"Values file sets",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ValuesFilesPaths,
			"values",
			[]string{},
			"Values files paths",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ValuesSets,
			"set",
			[]string{},
			"Values sets",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ValuesStringSets,
			"set-string",
			[]string{},
			"Values string sets",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ReleaseName,
			"release",
			"release-stub",
			"Release name",
			flag.AddOptions{
				ShortName: "r",
			},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
