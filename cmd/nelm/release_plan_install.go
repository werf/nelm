package main

import (
	"cmp"
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

type releasePlanInstallConfig struct {
	action.ReleasePlanInstallOptions

	LogColorMode     string
	LogLevel         string
	ReleaseName      string
	ReleaseNamespace string
}

func newReleasePlanInstallCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releasePlanInstallConfig{}

	use := "install [options...] -n namespace -r release"
	if featgate.FeatGateRemoteCharts.Enabled() || featgate.FeatGatePreviewV2.Enabled() {
		use += " [chart-dir|chart-repo-name/chart-name|chart-archive|chart-archive-url]"
	} else {
		use += " [chart-dir]"
	}

	cmd := cli.NewSubCommand(
		ctx,
		use,
		"Plan a release install to Kubernetes.",
		"Plan a release install to Kubernetes.",
		60,
		releaseCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveFilterDirs
			},
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = log.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), action.DefaultReleasePlanInstallLogLevel), log.SetupLoggingOptions{
				ColorMode: cfg.LogColorMode,
			})

			if len(args) > 0 {
				if featgate.FeatGateRemoteCharts.Enabled() || featgate.FeatGatePreviewV2.Enabled() {
					cfg.Chart = args[0]
				} else {
					cfg.ChartDirPath = args[0]
				}
			}

			if err := action.ReleasePlanInstall(ctx, cfg.ReleaseName, cfg.ReleaseNamespace, cfg.ReleasePlanInstallOptions); err != nil {
				return fmt.Errorf("release plan install: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := AddKubeConnectionFlags(cmd, &cfg.KubeConnectionOptions); err != nil {
			return fmt.Errorf("add kube connection flags: %w", err)
		}

		if err := AddChartRepoConnectionFlags(cmd, &cfg.ChartRepoConnectionOptions); err != nil {
			return fmt.Errorf("add chart repo connection flags: %w", err)
		}

		if err := AddValuesFlags(cmd, &cfg.ValuesOptions); err != nil {
			return fmt.Errorf("add values flags: %w", err)
		}

		if err := AddResourceValidationFlags(cmd, &cfg.ResourceValidationOptions); err != nil {
			return fmt.Errorf("add resource validation flags: %w", err)
		}

		if err := AddSecretValuesFlags(cmd, &cfg.SecretValuesOptions); err != nil {
			return fmt.Errorf("add secret values flags: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartAppVersion, "app-version", "", "Set appVersion of Chart.yaml", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartProvenanceKeyring, "provenance-keyring", "", "Path to keyring containing public keys to verify chart provenance", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO: restrict allowed values
		if err := cli.AddFlag(cmd, &cfg.ChartProvenanceStrategy, "provenance-strategy", common.DefaultChartProvenanceStrategy, "Strategy for provenance verifying", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepoSkipUpdate, "no-update-chart-repos", false, "Don't update chart repositories index", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if featgate.FeatGateRemoteCharts.Enabled() || featgate.FeatGatePreviewV2.Enabled() {
			if err := cli.AddFlag(cmd, &cfg.ChartVersion, "chart-version", "", "Choose a remote chart version, otherwise the latest version is used", cli.AddFlagOptions{
				GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
				Group:                mainFlagGroup,
			}); err != nil {
				return fmt.Errorf("add flag: %w", err)
			}
		}

		// TODO: restrict allowed values
		if err := cli.AddFlag(cmd, &cfg.DefaultDeletePropagation, "delete-propagation", string(common.DefaultDeletePropagation), "Default delete propagation strategy", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.DiffContextLines, "diff-context-lines", common.DefaultDiffContextLines, "Show N lines of context around diffs", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		var desc string
		if featgate.FeatGateMoreDetailedExitCodeForPlan.Enabled() || featgate.FeatGatePreviewV2.Enabled() {
			desc = "Return exit code 0 if no changes, 1 if error, 2 if resource changes planned, 3 if no resource changes planned, but release still should be installed"
		} else {
			desc = "Return exit code 0 if no changes, 1 if error, 2 if any changes planned"
		}

		if err := cli.AddFlag(cmd, &cfg.ErrorIfChangesPlanned, "exit-code", false, desc, cli.AddFlagOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraAnnotations, "annotations", map[string]string{}, "Add annotations to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraLabels, "labels", map[string]string{}, "Add labels to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Add annotations which will not trigger resource updates to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraRuntimeLabels, "runtime-labels", map[string]string{}, "Add labels which will not trigger resource updates to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ForceAdoption, "force-adoption", false, "Always adopt resources, even if they belong to a different Helm release", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.InstallGraphPath, "save-graph-to", "", "Save the Graphviz install graph to a file", cli.AddFlagOptions{
			Group: mainFlagGroup,
			Type:  cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NetworkParallelism, "network-parallelism", common.DefaultNetworkParallelism, "Limit of network-related tasks to run in parallel", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NoFinalTracking, "no-final-tracking", false, "By default disable tracking operations that have no create/update/delete resource operations after them, which are most tracking operations, to speed up the release", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                progressFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NoInstallStandaloneCRDs, "no-install-crds", false, `Don't install CRDs from "crds/" directories of installed charts`, cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NoRemoveManualChanges, "no-remove-manual-changes", false, "Don't remove fields added manually to the resource in the cluster if fields aren't present in the manifest", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.RegistryCredentialsPath, "oci-chart-repos-creds", common.DefaultRegistryCredentialsPath, "Credentials to access OCI chart repositories", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseInfoAnnotations, "release-info-annotations", map[string]string{}, "Add annotations to release metadata", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseLabels, "release-labels", map[string]string{}, "Add labels to the release. What kind of labels depends on the storage driver", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict allowed values
		if err := cli.AddFlag(cmd, &cfg.ReleaseStorageDriver, "release-storage", "", "How releases should be stored", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseStorageSQLConnection, "release-storage-sql-connection", "", "SQL connection string for MySQL release storage driver", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ShowInsignificantDiffs, "show-insignificant-diffs", false, "Show insignificant diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ShowSensitiveDiffs, "show-sensitive-diffs", false, "Show sensitive diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ShowVerboseCRDDiffs, "show-verbose-crd-diffs", false, "Show verbose CRD diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(v2): get rid?
		if err := cli.AddFlag(cmd, &cfg.ShowVerboseDiffs, "show-verbose-diffs", true, "Show verbose diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.PlanArtifactPath, "save-plan", "", "Save the gzip-compressed JSON install plan to the specified file", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TemplatesAllowDNS, "templates-allow-dns", false, "Allow performing DNS requests in templating", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.Timeout, "timeout", 0, "Fail if not finished in time", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", common.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(action.DefaultReleasePlanInstallLogLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseName, "release", "", "The release name. Must be unique within the release namespace", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
			ShortName:            "r",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseNamespace, "namespace", "", "The release namespace. Resources with no namespace will be deployed here", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
			ShortName:            "n",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
