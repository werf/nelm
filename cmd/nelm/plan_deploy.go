package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/flag"
)

type planDeployConfig struct {
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
	ErrorIfChangesPlanned        bool
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
	LogDebug                     bool
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	RegistryCredentialsPath      string
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         action.ReleaseStorageDriver
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SecretWorkDir                string
	TempDirPath                  string
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
}

func newPlanDeployCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &planDeployConfig{}

	cmd := &cobra.Command{
		Use:   "deploy [options...] -n namespace -r release [chart-dir]",
		Short: "Plan a release deployment to Kubernetes.",
		Long:  "Plan a release deployment to Kubernetes.",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				cfg.ChartDirPath = args[0]
			}

			if err := action.Plan(ctx, action.PlanOptions{
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
				ErrorIfChangesPlanned:        cfg.ErrorIfChangesPlanned,
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
				LogDebug:                     cfg.LogDebug,
				LogRegistryStreamOut:         cfg.LogRegistryStreamOut,
				NetworkParallelism:           cfg.NetworkParallelism,
				RegistryCredentialsPath:      cfg.RegistryCredentialsPath,
				ReleaseName:                  cfg.ReleaseName,
				ReleaseNamespace:             cfg.ReleaseNamespace,
				ReleaseStorageDriver:         cfg.ReleaseStorageDriver,
				SecretKeyIgnore:              cfg.SecretKeyIgnore,
				SecretValuesPaths:            cfg.SecretValuesPaths,
				SecretWorkDir:                cfg.SecretWorkDir,
				TempDirPath:                  cfg.TempDirPath,
				ValuesFileSets:               cfg.ValuesFileSets,
				ValuesFilesPaths:             cfg.ValuesFilesPaths,
				ValuesSets:                   cfg.ValuesSets,
				ValuesStringSets:             cfg.ValuesStringSets,
			}); err != nil {
				return fmt.Errorf("plan: %w", err)
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
			"Skip TLS verification for chart repository",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ChartRepositorySkipUpdate,
			"skip-dependency-update",
			false,
			"Skip update of the chart repository",
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
			&cfg.ErrorIfChangesPlanned,
			"exit-on-changes",
			false,
			"Exit with error if changes are planned",
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
			"Kube context to use",
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
			"Path to the registry credentials",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.ReleaseNamespace,
			"namespace",
			"",
			"Namespace for the release",
			flag.AddOptions{
				ShortName: "n",
				Required:  true,
			},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.SecretKeyIgnore,
			"ignore-secret-key",
			false,
			"Ignore secret keys",
			flag.AddOptions{},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(
			cmd,
			&cfg.SecretValuesPaths,
			"secret-values",
			[]string{},
			"Paths to secret values files",
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
			"Paths to values files\n(can be set multiple times)",
			flag.AddOptions{
				ShortName: "f",
			},
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
			"",
			"Release name",
			flag.AddOptions{
				ShortName: "r",
				Required:  true,
			},
		); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
