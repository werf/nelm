package commands

import (
	"context"
	"fmt"
	"time"
	"github.com/werf/logboek"

	"github.com/spf13/cobra"
	"github.com/werf/nelm/pkg/action"
)

func NewReleaseDeployCommand() *cobra.Command {
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

	f := cmd.Flags()
	// Define flags
	f.BoolVar(&opts.AutoRollback, "atomic", false, "Enable automatic rollback on failure")
	f.BoolVar(&opts.ChartRepositoryInsecure, "plain-http", false, "use insecure HTTP connections for the chart download")
	f.BoolVar(&opts.ChartRepositorySkipTLSVerify, "insecure-skip-tls-verify", false, "Skip TLS verification for chart repository")
	f.BoolVar(&opts.ChartRepositorySkipUpdate, "skip-dependency-update", false, "Skip update of the chart repository")
	f.BoolVar(&opts.DefaultSecretValuesDisable, "disable-default-secret-values", false, "Disable default secret values")
	f.BoolVar(&opts.DefaultValuesDisable, "disable-default-values", false, "Disable default values")
	f.StringVar(&opts.DeployGraphPath, "graph-path", "", "Path to save the deploy graph")
	f.BoolVar(&opts.DeployGraphSave, "graph", false, "Save the deploy graph")
	f.StringVar(&opts.DeployReportPath, "report-path", "", "Path to save the deploy report")
	f.BoolVar(&opts.DeployReportSave, "report", false, "Save the deploy report")
	f.StringToStringVarP(&opts.ExtraAnnotations, "annotations", "a", map[string]string{}, "Extra annotations to add to the rendered manifests")
	f.StringToStringVarP(&opts.ExtraLabels, "labels", "l", map[string]string{}, "Extra labels to add to the rendered manifests")
	f.StringToStringVar(&opts.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Extra runtime annotations to add to the rendered manifests")
	f.StringVar(&opts.KubeConfigBase64, "kubeconfig-base64", "", "Base64 encoded kube config")
	f.StringSliceVar(&opts.KubeConfigPaths, "kubeconfig", []string{}, "Paths to kube config files\n(can be set multiple times)")
	f.StringVar(&opts.KubeContext, "kube-context", "", "Kube context to use")
	f.IntVar(&opts.NetworkParallelism, "network-parallelism", 30, "Network parallelism")
	f.BoolVar(&opts.ProgressTablePrint, "kubedog", false, "Print progress table")
	f.DurationVar(&opts.ProgressTablePrintInterval, "kubedog-interval", 10*time.Second, "Progress table print interval")
	f.StringVar(&opts.RegistryCredentialsPath, "registry-credentials-path", "", "Path to the registry credentials")
	f.IntVar(&opts.ReleaseHistoryLimit, "history-max", 10, "The maximum number of revisions saved per release. Use 0 for no limit")
	f.StringVar(&opts.ReleaseNamespace, "namespace", "default", "Namespace for the release")
	f.StringVar(&opts.RollbackGraphPath, "rollback-graph-path", "", "Path to save the rollback graph")
	f.BoolVar(&opts.RollbackGraphSave, "rollback-graph", false, "Save the rollback graph")
	f.BoolVar(&opts.SecretKeyIgnore, "ignore-secret-key", false, "Ignore secret keys")
	f.StringSliceVar(&opts.SecretValuesPaths, "secret-values", []string{}, "Paths to secret values files")
	f.StringVar(&opts.TempDirPath, "temp-dir", "", "Path to the temporary directory")
	f.DurationVar(&opts.TrackCreationTimeout, "creation-timeout", 10*time.Minute, "Track creation timeout")
	f.DurationVar(&opts.TrackDeletionTimeout, "deletion-timeout", 10*time.Minute, "Track deletion timeout")
	f.DurationVar(&opts.TrackReadinessTimeout, "readiness-timeout", 10*time.Minute, "Track readiness timeout")
	f.StringSliceVar(&opts.ValuesFileSets, "set-file", []string{}, "Values file sets")
	f.StringSliceVarP(&opts.ValuesFilesPaths, "values", "f", []string{}, "Paths to values files\n(can be set multiple times)")
	f.StringSliceVar(&opts.ValuesSets, "set", []string{}, "Values sets")
	f.StringSliceVar(&opts.ValuesStringSets, "set-string", []string{}, "Values string sets")
	f.BoolVar(&opts.SubNotes, "render-subchart-notes", false, "Render subchart notes along with the parent")

	return cmd
}
