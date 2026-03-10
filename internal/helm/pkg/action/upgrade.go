/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package action

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/resource"

	"github.com/werf/nelm/internal/helm/pkg/chart"
	"github.com/werf/nelm/internal/helm/pkg/chartutil"
	"github.com/werf/nelm/internal/helm/pkg/kube"
	"github.com/werf/nelm/internal/helm/pkg/phases"
	"github.com/werf/nelm/internal/helm/pkg/phases/phasemanagers"
	"github.com/werf/nelm/internal/helm/pkg/phases/stages"
	"github.com/werf/nelm/internal/helm/pkg/postrender"
	"github.com/werf/nelm/internal/helm/pkg/registry"
	"github.com/werf/nelm/internal/helm/pkg/release"
	"github.com/werf/nelm/internal/helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/helm/pkg/storage/driver"
	"github.com/werf/nelm/internal/helm/pkg/werf/helmopts"
)

// Upgrade is the action for upgrading releases.
//
// It provides the implementation of 'helm upgrade'.
type Upgrade struct {
	cfg *Configuration

	ChartPathOptions

	// Install is a purely informative flag that indicates whether this upgrade was done in "install" mode.
	//
	// Applications may use this to determine whether this Upgrade operation was done as part of a
	// pure upgrade (Upgrade.Install == false) or as part of an install-or-upgrade operation
	// (Upgrade.Install == true).
	//
	// Setting this to `true` will NOT cause `Upgrade` to perform an install if the release does not exist.
	// That process must be handled by creating an Install action directly. See cmd/upgrade.go for an
	// example of how this flag is used.
	Install bool
	// Devel indicates that the operation is done in devel mode.
	Devel bool
	// Namespace is the namespace in which this operation should be performed.
	Namespace string
	// SkipCRDs skips installing CRDs when install flag is enabled during upgrade
	SkipCRDs bool
	// Timeout is the timeout for this operation
	Timeout time.Duration
	// Wait determines whether the wait operation should be performed after the upgrade is requested.
	Wait bool
	// WaitForJobs determines whether the wait operation for the Jobs should be performed after the upgrade is requested.
	WaitForJobs bool
	// DisableHooks disables hook processing if set to true.
	DisableHooks bool
	// DryRun controls whether the operation is prepared, but not executed.
	DryRun bool
	// DryRunOption controls whether the operation is prepared, but not executed with options on whether or not to interact with the remote cluster.
	DryRunOption string
	// Force will, if set to `true`, ignore certain warnings and perform the upgrade anyway.
	//
	// This should be used with caution.
	Force bool
	// ResetValues will reset the values to the chart's built-ins rather than merging with existing.
	ResetValues bool
	// ReuseValues will re-use the user's last supplied values.
	ReuseValues bool
	// ResetThenReuseValues will reset the values to the chart's built-ins then merge with user's last supplied values.
	ResetThenReuseValues bool
	// Recreate will (if true) recreate pods after a rollback.
	Recreate bool
	// MaxHistory limits the maximum number of revisions saved per release
	MaxHistory int
	// Atomic, if true, will roll back on failure.
	Atomic bool
	// CleanupOnFail will, if true, cause the upgrade to delete newly-created resources on a failed update.
	CleanupOnFail bool
	// SubNotes determines whether sub-notes are rendered in the chart.
	SubNotes bool
	// Description is the description of this operation
	Description string
	Labels      map[string]string
	// PostRender is an optional post-renderer
	//
	// If this is non-nil, then after templates are rendered, they will be sent to the
	// post renderer before sending to the Kubernetes API server.
	PostRenderer postrender.PostRenderer
	// DisableOpenAPIValidation controls whether OpenAPI validation is enforced.
	DisableOpenAPIValidation bool
	// Get missing dependencies
	DependencyUpdate bool
	// Lock to control raceconditions when the process receives a SIGTERM
	Lock sync.Mutex
	// Enable DNS lookups when rendering templates
	EnableDNS bool

	DeployReportPath            string
	StagesSplitter              phases.Splitter
	StagesExternalDepsGenerator phases.ExternalDepsGenerator
	IgnorePending               bool
}

type resultMessage struct {
	r *release.Release
	e error
}

type UpgradeOptions struct {
	StagesSplitter              phases.Splitter
	StagesExternalDepsGenerator phases.ExternalDepsGenerator
	IgnorePending               bool
}

// NewUpgrade creates a new Upgrade object with the given configuration.
func NewUpgrade(cfg *Configuration, opts UpgradeOptions) *Upgrade {
	stagesSplitter := opts.StagesSplitter
	if stagesSplitter == nil {
		stagesSplitter = &phases.SingleStageSplitter{}
	}

	stagesExternalDepsGenerator := opts.StagesExternalDepsGenerator
	if stagesExternalDepsGenerator == nil {
		stagesExternalDepsGenerator = &phases.NoExternalDepsGenerator{}
	}

	up := &Upgrade{
		cfg: cfg,

		StagesSplitter:              stagesSplitter,
		StagesExternalDepsGenerator: stagesExternalDepsGenerator,
		IgnorePending:               opts.IgnorePending,
	}
	up.ChartPathOptions.registryClient = cfg.RegistryClient

	return up
}

// SetRegistryClient sets the registry client to use when fetching charts.
func (u *Upgrade) SetRegistryClient(client *registry.Client) {
	u.ChartPathOptions.registryClient = client
}

// Run executes the upgrade on the given release.
func (u *Upgrade) Run(name string, chart *chart.Chart, vals map[string]interface{}) (*release.Release, error) {
	ctx := context.Background()
	return u.RunWithContext(ctx, name, chart, vals)
}

// RunWithContext executes the upgrade on the given release with context.
func (u *Upgrade) RunWithContext(ctx context.Context, name string, chart *chart.Chart, vals map[string]interface{}) (*release.Release, error) {
	if err := u.cfg.KubeClient.IsReachable(); err != nil {
		return nil, err
	}

	// Make sure if Atomic is set, that wait is set as well. This makes it so
	// the user doesn't have to specify both
	u.Wait = u.Wait || u.Atomic

	if err := chartutil.ValidateReleaseName(name); err != nil {
		return nil, errors.Errorf("release name is invalid: %s", name)
	}

	u.cfg.Log("preparing upgrade for %s", name)
	currentRelease, upgradedRelease, err := u.prepareUpgrade(name, chart, vals)
	if err != nil {
		return nil, err
	}

	if !u.isDryRun() && u.DeployReportPath != "" {
		defer func() {
			deployReportData, err := release.NewDeployReport().FromRelease(upgradedRelease).ToJSONData()
			if err != nil {
				u.cfg.Log("warning: error creating deploy report data: %s", err)
				return
			}

			if err := os.WriteFile(u.DeployReportPath, deployReportData, 0o644); err != nil {
				u.cfg.Log("warning: error writing deploy report file: %s", err)
				return
			}
		}()
	}

	u.cfg.Releases.MaxHistory = u.MaxHistory

	u.cfg.Log("performing update for %s", name)
	res, err := u.performUpgrade(ctx, currentRelease, upgradedRelease)
	if err != nil {
		return res, err
	}

	// Do not update for dry runs
	if !u.isDryRun() {
		u.cfg.Log("updating status for upgraded release for %s", name)
		if err := u.cfg.Releases.Update(upgradedRelease); err != nil {
			return res, err
		}
	}

	return res, nil
}

// isDryRun returns true if Upgrade is set to run as a DryRun
func (u *Upgrade) isDryRun() bool {
	if u.DryRun || u.DryRunOption == "client" || u.DryRunOption == "server" || u.DryRunOption == "true" {
		return true
	}
	return false
}

// prepareUpgrade builds an upgraded release for an upgrade operation.
func (u *Upgrade) prepareUpgrade(name string, chart *chart.Chart, vals map[string]interface{}) (*release.Release, *release.Release, error) {
	if chart == nil {
		return nil, nil, errMissingChart
	}

	// finds the last non-deleted release with the given name
	lastRelease, err := u.cfg.Releases.Last(name)
	if err != nil {
		// to keep existing behavior of returning the "%q has no deployed releases" error when an existing release does not exist
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return nil, nil, driver.NewErrNoDeployedReleases(name)
		}
		return nil, nil, err
	}

	isUpgrade := true

	var currentRelease *release.Release
	if lastRelease.Info.Status == release.StatusDeployed {
		// no need to retrieve the last deployed release from storage as the last release is deployed
		currentRelease = lastRelease
	} else {
		// finds the deployed release with the given name
		currentRelease, err = u.cfg.Releases.Deployed(name)
		if err != nil {
			if errors.Is(err, driver.ErrNoDeployedReleases) && lastRelease.Info.Status != release.StatusUnknown {
				switch lastRelease.Info.Status {
				case release.StatusUninstalling, release.StatusUninstalled, release.StatusPendingInstall:
					isUpgrade = false
				}
				currentRelease = lastRelease
			} else {
				return nil, nil, err
			}
		}
	}

	// determine if values will be reused
	vals, err = u.reuseValues(chart, currentRelease, vals)
	if err != nil {
		return nil, nil, err
	}

	if err := chartutil.ProcessDependenciesWithMerge(chart, &vals); err != nil {
		return nil, nil, err
	}

	// Increment revision count. This is passed to templates, and also stored on
	// the release object.
	revision := lastRelease.Version + 1

	options := chartutil.ReleaseOptions{
		Name:      name,
		Namespace: currentRelease.Namespace,
		Revision:  revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}

	caps, err := u.cfg.getCapabilities()
	if err != nil {
		return nil, nil, err
	}
	valuesToRender, err := chartutil.ToRenderValues(chart, vals, options, caps, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	// Determine whether or not to interact with remote
	var interactWithRemote bool
	if !u.isDryRun() || u.DryRunOption == "server" || u.DryRunOption == "none" || u.DryRunOption == "false" {
		interactWithRemote = true
	}

	hooks, manifestDoc, notesTxt, err := u.cfg.renderResources(chart, valuesToRender, "", "", u.SubNotes, false, false, u.PostRenderer, interactWithRemote, u.EnableDNS, helmopts.HelmOptions{})
	if err != nil {
		return nil, nil, err
	}

	if driver.ContainsSystemLabels(u.Labels) {
		return nil, nil, fmt.Errorf("user suplied labels contains system reserved label name. System labels: %+v", driver.GetSystemLabels())
	}

	// Store an upgraded release.
	upgradedRelease := release.SetInitPhaseStageInfo(&release.Release{
		Name:      name,
		Namespace: currentRelease.Namespace,
		Chart:     chart,
		Config:    vals,
		Info: &release.Info{
			FirstDeployed: currentRelease.Info.FirstDeployed,
			LastDeployed:  Timestamper(),
			Status:        release.StatusPendingUpgrade,
			Description:   "Preparing upgrade", // This should be overwritten later.
		},
		Version:  revision,
		Manifest: manifestDoc.String(),
		Hooks:    hooks,
		Labels:   mergeCustomLabels(lastRelease.Labels, u.Labels),
	})

	if len(notesTxt) > 0 {
		upgradedRelease.Info.Notes = notesTxt
	}
	err = validateManifest(u.cfg.KubeClient, manifestDoc.Bytes(), !u.DisableOpenAPIValidation)
	return currentRelease, upgradedRelease, err
}

func (u *Upgrade) performUpgrade(ctx context.Context, originalRelease, upgradedRelease *release.Release) (*release.Release, error) {
	current, err := u.cfg.KubeClient.Build(bytes.NewBufferString(originalRelease.Manifest), false)
	if err != nil {
		// Checking for removed Kubernetes API error so can provide a more informative error message to the user
		// Ref: https://github.com/helm/helm/issues/7219
		if strings.Contains(err.Error(), "unable to recognize \"\": no matches for kind") {
			return upgradedRelease, errors.Wrap(err, "current release manifest contains removed kubernetes api(s) for this "+
				"kubernetes version and it is therefore unable to build the kubernetes "+
				"objects for performing the diff. error from kubernetes; "+fmt.Sprintf("unable to load original manifests:\n%s\n---\nupgraded release manifests:\n%s\n---\nPlease report to https://github.com/werf/werf/issues if this error have occured for an actual api version", originalRelease.Manifest, upgradedRelease.Manifest))

		}
		return upgradedRelease, errors.Wrap(err, "unable to build kubernetes objects from current release manifest")
	}
	target, err := u.cfg.KubeClient.Build(bytes.NewBufferString(upgradedRelease.Manifest), !u.DisableOpenAPIValidation)
	if err != nil {
		return upgradedRelease, errors.Wrap(err, "unable to build kubernetes objects from new release manifest")
	}

	// It is safe to use force only on target because these are resources currently rendered by the chart.
	err = target.Visit(releaseutil.SetMetadataVisitor(upgradedRelease.Name, upgradedRelease.Namespace, true))
	if err != nil {
		return upgradedRelease, err
	}

	// Do a basic diff using gvk + name to figure out what new resources are being created so we can validate they don't already exist
	existingResources := make(map[string]bool)
	for _, r := range current {
		existingResources[objectKey(r)] = true
	}

	var toBeCreated kube.ResourceList
	for _, r := range target {
		if !existingResources[objectKey(r)] {
			toBeCreated = append(toBeCreated, r)
		}
	}

	toBeAdopted, err := existingResourceConflict(toBeCreated, upgradedRelease.Name, upgradedRelease.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to continue with update")
	}

	// Run if it is a dry run
	if u.isDryRun() {
		u.cfg.Log("dry run for %s", upgradedRelease.Name)
		if len(u.Description) > 0 {
			upgradedRelease.Info.Description = u.Description
		} else {
			upgradedRelease.Info.Description = "Dry run complete"
		}
		return upgradedRelease, nil
	}

	u.cfg.Log("creating upgraded release for %s", upgradedRelease.Name)
	if err := u.cfg.Releases.Create(upgradedRelease); err != nil {
		return nil, err
	}
	rChan := make(chan resultMessage)
	ctxChan := make(chan resultMessage)
	doneChan := make(chan interface{})
	defer close(doneChan)
	go u.releasingUpgrade(rChan, upgradedRelease, toBeAdopted, target, originalRelease)
	go u.handleContext(ctx, doneChan, ctxChan, upgradedRelease)
	select {
	case result := <-rChan:
		return result.r, result.e
	case result := <-ctxChan:
		return result.r, result.e
	}
}

// Function used to lock the Mutex, this is important for the case when the atomic flag is set.
// In that case the upgrade will finish before the rollback is finished so it is necessary to wait for the rollback to finish.
// The rollback will be trigger by the function failRelease
func (u *Upgrade) reportToPerformUpgrade(c chan<- resultMessage, rel *release.Release, created kube.ResourceList, err error) {
	u.Lock.Lock()
	if err != nil {
		rel, err = u.failRelease(rel, created, err)
	}
	c <- resultMessage{r: rel, e: err}
	u.Lock.Unlock()
}

// Setup listener for SIGINT and SIGTERM
func (u *Upgrade) handleContext(ctx context.Context, done chan interface{}, c chan<- resultMessage, upgradedRelease *release.Release) {
	select {
	case <-ctx.Done():
		err := ctx.Err()

		// when the atomic flag is set the ongoing release finish first and doesn't give time for the rollback happens.
		u.reportToPerformUpgrade(c, upgradedRelease, kube.ResourceList{}, err)
	case <-done:
		return
	}
}
func (u *Upgrade) releasingUpgrade(c chan<- resultMessage, upgradedRelease *release.Release, toBeAdopted kube.ResourceList, target kube.ResourceList, originalRelease *release.Release) {
	// pre-upgrade hooks

	if !u.DisableHooks {
		if err := u.cfg.execHook(upgradedRelease, release.HookPreUpgrade, u.Timeout); err != nil {
			u.reportToPerformUpgrade(c, upgradedRelease, kube.ResourceList{}, fmt.Errorf("pre-upgrade hooks failed: %s", err))
			return
		}
	} else {
		u.cfg.Log("upgrade hooks disabled for %s", upgradedRelease.Name)
	}

	history, err := u.cfg.Releases.HistoryUntilRevision(upgradedRelease.Name, upgradedRelease.Version)
	if err != nil {
		u.cfg.recordRelease(originalRelease)
		u.reportToPerformUpgrade(c, upgradedRelease, kube.ResourceList{}, fmt.Errorf("error getting release history: %w", err))
		return
	}

	rolloutPhase, err := phases.NewRolloutPhase(upgradedRelease, u.StagesSplitter, u.cfg.KubeClient).
		ParseStages(target)
	if err != nil {
		u.cfg.recordRelease(originalRelease)
		u.reportToPerformUpgrade(c, upgradedRelease, kube.ResourceList{}, fmt.Errorf("error parsing stages for rollout phase: %w", err))
		return
	}

	if err := rolloutPhase.GenerateStagesExternalDeps(u.StagesExternalDepsGenerator); err != nil {
		u.cfg.recordRelease(originalRelease)
		u.reportToPerformUpgrade(c, upgradedRelease, kube.ResourceList{}, fmt.Errorf("error generating external deps for rollout phase: %w", err))
		return
	}

	deployedResourcesCalculator := phases.NewDeployedResourcesCalculator(history, u.StagesSplitter, u.cfg.KubeClient)

	rolloutPhaseManager, err := phasemanagers.NewRolloutPhaseManager(rolloutPhase, deployedResourcesCalculator, upgradedRelease, u.cfg.Releases, u.cfg.KubeClient).
		AddPreviouslyDeployedResources(toBeAdopted).
		AddCalculatedPreviouslyDeployedResources()
	if err != nil {
		u.cfg.recordRelease(originalRelease)
		u.reportToPerformUpgrade(c, upgradedRelease, kube.ResourceList{}, fmt.Errorf("error calculating previously deployed resources for rollout phase manager: %w", err))
		return
	}

	if err := rolloutPhaseManager.DoStage(
		func(stgIndex int, stage *stages.Stage) error {
			if len(stage.ExternalDependencies) == 0 || !u.Wait {
				return nil
			}

			if u.WaitForJobs {
				return u.cfg.KubeClient.WaitWithJobs(stage.ExternalDependencies.AsResourceList(), u.Timeout)
			} else {
				return u.cfg.KubeClient.Wait(stage.ExternalDependencies.AsResourceList(), u.Timeout)
			}
		},
		func(stgIndex int, stage *stages.Stage, prevDeployedStgResources kube.ResourceList) error {
			if len(prevDeployedStgResources) == 0 {
				stage.Result, err = u.cfg.KubeClient.Create(stage.DesiredResources, kube.CreateOptions{})
				if err != nil {
					return err
				}
			} else {
				stage.Result, err = u.cfg.KubeClient.Update(prevDeployedStgResources, stage.DesiredResources, u.Force, kube.UpdateOptions{
					SkipDeleteIfInvalidOwnership: true,
					ReleaseName:                  upgradedRelease.Name,
					ReleaseNamespace:             upgradedRelease.Namespace,
				})
				if err != nil {
					return err
				}
			}

			if u.Recreate {
				// NOTE: Because this is not critical for a release to succeed, we just
				// log if an error occurs and continue onward. If we ever introduce log
				// levels, we should make these error level logs so users are notified
				// that they'll need to go do the cleanup on their own
				if err := recreate(u.cfg, stage.Result.Updated); err != nil {
					u.cfg.Log(err.Error())
				}
			}

			return nil
		},
		func(stgIndex int, stage *stages.Stage) error {
			if !u.Wait {
				return nil
			}

			if u.WaitForJobs {
				return u.cfg.KubeClient.WaitWithJobs(stage.DesiredResources, u.Timeout)
			} else {
				return u.cfg.KubeClient.Wait(stage.DesiredResources, u.Timeout)
			}
		},
	); err != nil {
		u.cfg.recordRelease(originalRelease)

		createdResourcesToDelete := kube.ResourceList{}
		var applyErr *phasemanagers.ApplyError
		if errors.As(err, &applyErr) {
			createdResourcesToDelete = rolloutPhaseManager.Phase.SortedStages[applyErr.StageIndex].Result.Created
		}

		u.reportToPerformUpgrade(c, upgradedRelease, createdResourcesToDelete, fmt.Errorf("error processing rollout phase stage: %w", err))

		return
	}

	if err := rolloutPhaseManager.DeleteOrphanedResources(); err != nil {
		u.cfg.Log("failure removing resources no longer present in the release: %w", err)
	}

	// post-upgrade hooks
	if !u.DisableHooks {
		if err := u.cfg.execHook(upgradedRelease, release.HookPostUpgrade, u.Timeout); err != nil {
			u.reportToPerformUpgrade(c, upgradedRelease, rolloutPhaseManager.Phase.SortedStages.MergedCreatedResources(), fmt.Errorf("post-upgrade hooks failed: %s", err))
			return
		}
	}

	originalRelease.Info.Status = release.StatusSuperseded
	u.cfg.recordRelease(originalRelease)

	upgradedRelease.Info.Status = release.StatusDeployed
	if len(u.Description) > 0 {
		upgradedRelease.Info.Description = u.Description
	} else {
		upgradedRelease.Info.Description = "Upgrade complete"
	}
	u.reportToPerformUpgrade(c, upgradedRelease, nil, nil)
}

func (u *Upgrade) failRelease(rel *release.Release, created kube.ResourceList, err error) (*release.Release, error) {
	msg := fmt.Sprintf("Upgrade %q failed: %s", rel.Name, err)
	u.cfg.Log("warning: %s", msg)

	rel.Info.Status = release.StatusFailed
	rel.Info.Description = msg
	u.cfg.recordRelease(rel)
	if u.CleanupOnFail && len(created) > 0 {
		u.cfg.Log("Cleanup on fail set, cleaning up %d resources", len(created))
		_, errs := u.cfg.KubeClient.Delete(created, kube.DeleteOptions{
			Wait:                   true,
			WaitTimeout:            u.Timeout,
			SkipIfInvalidOwnership: true,
			ReleaseName:            rel.Name,
			ReleaseNamespace:       rel.Namespace,
		})
		if errs != nil {
			var errorList []string
			for _, e := range errs {
				errorList = append(errorList, e.Error())
			}
			return rel, errors.Wrapf(fmt.Errorf("unable to cleanup resources: %s", strings.Join(errorList, ", ")), "an error occurred while cleaning up resources. original upgrade error: %s", err)
		}
		u.cfg.Log("Resource cleanup complete")
	}
	if u.Atomic {
		u.cfg.Log("Upgrade failed and atomic is set, rolling back to last successful release")

		// As a protection, get the last successful release before rollback.
		// If there are no successful releases, bail out
		hist := NewHistory(u.cfg)
		fullHistory, herr := hist.Run(rel.Name)
		if herr != nil {
			return rel, errors.Wrapf(herr, "an error occurred while finding last successful release. original upgrade error: %s", err)
		}

		// There isn't a way to tell if a previous release was successful, but
		// generally failed releases do not get superseded unless the next
		// release is successful, so this should be relatively safe
		filteredHistory := releaseutil.FilterFunc(func(r *release.Release) bool {
			return r.Info.Status == release.StatusSuperseded || r.Info.Status == release.StatusDeployed
		}).Filter(fullHistory)
		if len(filteredHistory) == 0 {
			return rel, errors.Wrap(err, "unable to find a previously successful release when attempting to rollback. original upgrade error")
		}

		releaseutil.Reverse(filteredHistory, releaseutil.SortByRevision)

		rollin := NewRollback(u.cfg, u.StagesSplitter, u.StagesExternalDepsGenerator)
		rollin.Version = filteredHistory[0].Version
		rollin.Wait = true
		rollin.WaitForJobs = u.WaitForJobs
		rollin.DisableHooks = u.DisableHooks
		rollin.Recreate = u.Recreate
		rollin.Force = u.Force
		rollin.Timeout = u.Timeout

		rollin.CleanupOnFail = u.CleanupOnFail

		if rollErr := rollin.Run(rel.Name); rollErr != nil {
			return rel, errors.Wrapf(rollErr, "an error occurred while rolling back the release. original upgrade error: %s", err)
		}
		return rel, errors.Wrapf(err, "release %s failed, and has been rolled back due to atomic being set", rel.Name)
	}

	return rel, err
}

// reuseValues copies values from the current release to a new release if the
// new release does not have any values.
//
// If the request already has values, or if there are no values in the current
// release, this does nothing.
//
// This is skipped if the u.ResetValues flag is set, in which case the
// request values are not altered.
func (u *Upgrade) reuseValues(chart *chart.Chart, current *release.Release, newVals map[string]interface{}) (map[string]interface{}, error) {
	if u.ResetValues {
		// If ResetValues is set, we completely ignore current.Config.
		u.cfg.Log("resetting values to the chart's original version")
		return newVals, nil
	}

	// If the ReuseValues flag is set, we always copy the old values over the new config's values.
	if u.ReuseValues {
		u.cfg.Log("reusing the old release's values")

		// We have to regenerate the old coalesced values:
		oldVals, err := chartutil.CoalesceValues(current.Chart, current.Config)
		if err != nil {
			return nil, errors.Wrap(err, "failed to rebuild old values")
		}

		newVals = chartutil.CoalesceTables(newVals, current.Config)

		chart.Values = oldVals

		return newVals, nil
	}

	// If the ResetThenReuseValues flag is set, we use the new chart's values, but we copy the old config's values over the new config's values.
	if u.ResetThenReuseValues {
		u.cfg.Log("merging values from old release to new values")

		newVals = chartutil.CoalesceTables(newVals, current.Config)

		return newVals, nil
	}

	if len(newVals) == 0 && len(current.Config) > 0 {
		u.cfg.Log("copying values from %s (v%d) to new release.", current.Name, current.Version)
		newVals = current.Config
	}
	return newVals, nil
}

func validateManifest(c kube.Interface, manifest []byte, openAPIValidation bool) error {
	_, err := c.Build(bytes.NewReader(manifest), openAPIValidation)
	return err
}

// recreate captures all the logic for recreating pods for both upgrade and
// rollback. If we end up refactoring rollback to use upgrade, this can just be
// made an unexported method on the upgrade action.
func recreate(cfg *Configuration, resources kube.ResourceList) error {
	for _, res := range resources {
		versioned := kube.AsVersioned(res)
		selector, err := kube.SelectorsForObject(versioned)
		if err != nil {
			// If no selector is returned, it means this object is
			// definitely not a pod, so continue onward
			continue
		}

		client, err := cfg.KubernetesClientSet()
		if err != nil {
			return errors.Wrapf(err, "unable to recreate pods for object %s/%s because an error occurred", res.Namespace, res.Name)
		}

		pods, err := client.CoreV1().Pods(res.Namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		if err != nil {
			return errors.Wrapf(err, "unable to recreate pods for object %s/%s because an error occurred", res.Namespace, res.Name)
		}

		// Restart pods
		for _, pod := range pods.Items {
			// Delete each pod for get them restarted with changed spec.
			if err := client.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, *metav1.NewPreconditionDeleteOptions(string(pod.UID))); err != nil {
				return errors.Wrapf(err, "unable to recreate pods for object %s/%s because an error occurred", res.Namespace, res.Name)
			}
		}
	}
	return nil
}

func objectKey(r *resource.Info) string {
	gvk := r.Object.GetObjectKind().GroupVersionKind()
	return fmt.Sprintf("%s/%s/%s/%s", gvk.GroupVersion().String(), gvk.Kind, r.Namespace, r.Name)
}

func mergeCustomLabels(current, desired map[string]string) map[string]string {
	labels := mergeStrStrMaps(current, desired)
	for k, v := range labels {
		if v == "null" {
			delete(labels, k)
		}
	}
	return labels
}

// merge two maps, always taking the value on the right
func mergeStrStrMaps(current, desired map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range current {
		result[k] = v
	}
	for k, desiredVal := range desired {
		result[k] = desiredVal
	}
	return result
}
