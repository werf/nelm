package commands

import (
	"context"
	"fmt"
	"time"
	"github.com/werf/logboek"

	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/action"
)

func NewDeployCommand() *cobra.Command {
	var opts action.DeployOptions

	cmd := &cobra.Command{
		Use:     "deploy [release-name] [chart-dir]",
		Short:   "Deploy a Helm chart",
		Long:    "Deploy a Helm chart with the specified release name.",
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
			if err := action.Deploy(ctx, opts); err != nil {
				return fmt.Errorf("deploy failed: %w", err)
			}
			return nil
		},
	}

	// Define flags
	cmd.Flags().BoolVar(&opts.AutoRollback, "atomic", false, "Enable automatic rollback on failure")
	cmd.Flags().BoolVar(&opts.ChartRepositoryInsecure, "plain-http", false, "use insecure HTTP connections for the chart download")
	cmd.Flags().BoolVar(&opts.ChartRepositorySkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS verification for chart repository")
	cmd.Flags().BoolVar(&opts.ChartRepositorySkipUpdate, "skip-dependency-update", false, "Skip update of the chart repository")
	cmd.Flags().BoolVar(&opts.DefaultSecretValuesDisable, "disable-default-secret-values", false, "Disable default secret values")
	cmd.Flags().BoolVar(&opts.DefaultValuesDisable, "disable-default-values", false, "Disable default values")
	cmd.Flags().StringVar(&opts.DeployGraphPath, "graph-path", "", "Path to save the deploy graph")
	cmd.Flags().BoolVar(&opts.DeployGraphSave, "graph", false, "Save the deploy graph")
	cmd.Flags().StringVar(&opts.DeployReportPath, "report-path", "", "Path to save the deploy report")
	cmd.Flags().BoolVar(&opts.DeployReportSave, "report", false, "Save the deploy report")
	cmd.Flags().StringToStringVarP(&opts.ExtraAnnotations, "annotations", "a", map[string]string{}, "Extra annotations to add to the rendered manifests")
	cmd.Flags().StringToStringVarP(&opts.ExtraLabels, "labels", "l", map[string]string{}, "Extra labels to add to the rendered manifests")
	cmd.Flags().StringToStringVar(&opts.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Extra runtime annotations to add to the rendered manifests")
	cmd.Flags().StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	cmd.Flags().StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	cmd.Flags().StringVar(&opts.KubeContext, "kube-context", "", "Kube context to use")
	cmd.Flags().IntVar(&opts.NetworkParallelism, "network-parallelism", 30, "Network parallelism")
	cmd.Flags().BoolVar(&opts.ProgressTablePrint, "kubedog", false, "Print progress table")
	cmd.Flags().DurationVar(&opts.ProgressTablePrintInterval, "kubedog-interval", 10*time.Second, "Progress table print interval")
	cmd.Flags().StringVar(&opts.RegistryCredentialsPath, "registry-credentials-path", "", "Path to the registry credentials")
	cmd.Flags().IntVar(&opts.ReleaseHistoryLimit, "history-max", 10, "The maximum number of revisions saved per release. Use 0 for no limit")
	cmd.Flags().StringVar(&opts.ReleaseNamespace, "namespace", "default", "Namespace for the release")
	cmd.Flags().StringVar(&opts.RollbackGraphPath, "rollback-graph-path", "", "Path to save the rollback graph")
	cmd.Flags().BoolVar(&opts.RollbackGraphSave, "rollback-graph", false, "Save the rollback graph")
	cmd.Flags().BoolVar(&opts.SecretKeyIgnore, "ignore-secret-key", false, "Ignore secret keys")
	cmd.Flags().StringSliceVar(&opts.SecretValuesPaths, "secret-values", []string{}, "Paths to secret values files")
	cmd.Flags().StringVar(&opts.TempDirPath, "temp-dir", "", "Path to the temporary directory")
	cmd.Flags().DurationVar(&opts.TrackCreationTimeout, "creation-timeout", 10*time.Minute, "Track creation timeout")
	cmd.Flags().DurationVar(&opts.TrackDeletionTimeout, "deletion-timeout", 10*time.Minute, "Track deletion timeout")
	cmd.Flags().DurationVar(&opts.TrackReadinessTimeout, "readiness-timeout", 10*time.Minute, "Track readiness timeout")
	cmd.Flags().StringSliceVar(&opts.ValuesFileSets, "set-file", []string{}, "Values file sets")
	cmd.Flags().StringSliceVarP(&opts.ValuesFilesPaths, "values", "f", []string{}, "Paths to values files\n(can be set multiple times)")
	cmd.Flags().StringSliceVar(&opts.ValuesSets, "set", []string{}, "Values sets")
	cmd.Flags().StringSliceVar(&opts.ValuesStringSets, "set-string", []string{}, "Values string sets")
	cmd.Flags().BoolVar(&opts.SubNotes, "render-subchart-notes", false, "Render subchart notes along with the parent")

	return cmd
}
