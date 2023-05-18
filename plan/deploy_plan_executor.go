package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/werf/client"
	"helm.sh/helm/v3/pkg/werf/resource"
	"helm.sh/helm/v3/pkg/werf/resourcetracker"
)

func NewDeployPlanExecutor(plan *Plan, releaseNs *resource.UnmanagedResource, cli *client.Client, track *resourcetracker.ResourceTracker, releaseStorage *storage.Storage) *DeployPlanExecutor {
	return &DeployPlanExecutor{
		plan:             plan,
		releaseNamespace: releaseNs,
		client:           cli,
		tracker:          track,
		releaseStorage:   releaseStorage,
	}
}

type DeployPlanExecutor struct {
	plan             *Plan
	report           *Report
	releaseNamespace *resource.UnmanagedResource
	trackTimeout     time.Duration

	client         *client.Client
	tracker        *resourcetracker.ResourceTracker
	releaseStorage *storage.Storage
}

func (e *DeployPlanExecutor) WithTrackTimeout(timeout time.Duration) *DeployPlanExecutor {
	e.trackTimeout = timeout
	return e
}

func (e *DeployPlanExecutor) WithReport(report *Report) *DeployPlanExecutor {
	e.report = report
	return e
}

func (e *DeployPlanExecutor) Execute(ctx context.Context) (*Report, error) {
	if e.report == nil {
		e.report = NewReport()
	}

	for _, phase := range e.plan.Phases {
		for _, operation := range phase.Operations {
			switch op := operation.(type) {
			case *OperationCreate:
				createResult, errs := e.client.Create(ctx, client.CreateOptions{FallbackNamespace: e.releaseNamespace.Name()}, op.Targets...)
				for _, res := range createResult.Created {
					createdRes := struct {
						Target resource.Referencer
						Result *resource.GenericResource
					}{
						Target: res.Target,
						Result: res.Result,
					}
					e.report.Created = append(e.report.Created, createdRes)
				}
				if errs != nil {
					return e.report, fmt.Errorf("error creating resources: %w", multierror.Append(nil, errs...))
				}
			case *OperationRecreate:
				createResult, errs := e.client.Create(ctx, client.CreateOptions{FallbackNamespace: e.releaseNamespace.Name(), Recreate: true}, op.Targets...)
				for _, res := range createResult.Created {
					createdRes := struct {
						Target resource.Referencer
						Result *resource.GenericResource
					}{
						Target: res.Target,
						Result: res.Result,
					}
					e.report.Created = append(e.report.Created, createdRes)
				}
				for _, res := range createResult.Recreated {
					recreatedRes := struct {
						Target resource.Referencer
						Result *resource.GenericResource
					}{
						Target: res.Target,
						Result: res.Result,
					}
					e.report.Recreated = append(e.report.Recreated, recreatedRes)
				}
				if errs != nil {
					return e.report, fmt.Errorf("error creating resources: %w", multierror.Append(nil, errs...))
				}
			case *OperationUpdate:
				updateResult, errs := e.client.Update(ctx, client.UpdateOptions{FallbackNamespace: e.releaseNamespace.Name()}, op.Targets...)
				for _, res := range updateResult.Updated {
					updatedRes := struct {
						Target resource.Referencer
						Result *resource.GenericResource
					}{
						Target: res.Target,
						Result: res.Result,
					}
					e.report.Updated = append(e.report.Updated, updatedRes)
				}
				if errs != nil {
					return e.report, fmt.Errorf("error updating resources: %w", multierror.Append(nil, errs...))
				}
			case *OperationDelete:
				deleteResult, errs := e.client.Delete(ctx, client.DeleteOptions{FallbackNamespace: e.releaseNamespace.Name(), ContinueOnError: true}, op.Targets...)
				e.report.Deleted = append(e.report.Deleted, deleteResult.Deleted...)
				if errs != nil {
					return e.report, fmt.Errorf("error deleting resources: %w", multierror.Append(nil, errs...))
				}
			case *OperationTrackHelmHooksReadiness:
				if err := e.tracker.TrackHelmHooksReadiness(ctx, resourcetracker.TrackHelmHooksReadinessOptions{FallbackNamespace: e.releaseNamespace.Name(), Timeout: e.trackTimeout}, op.Targets...); err != nil {
					return e.report, fmt.Errorf("error tracking helm hooks readiness: %w", err)
				}
			case *OperationTrackHelmResourcesReadiness:
				if err := e.tracker.TrackHelmResourcesReadiness(ctx, resourcetracker.TrackHelmResourcesReadinessOptions{FallbackNamespace: e.releaseNamespace.Name(), Timeout: e.trackTimeout}, op.Targets...); err != nil {
					return e.report, fmt.Errorf("error tracking helm resources readiness: %w", err)
				}
			case *OperationTrackUnmanagedResourcesReadiness:
				if err := e.tracker.TrackUnmanagedResourcesReadiness(ctx, resourcetracker.TrackUnmanagedResourcesReadinessOptions{FallbackNamespace: e.releaseNamespace.Name(), Timeout: e.trackTimeout}, op.Targets...); err != nil {
					return e.report, fmt.Errorf("error tracking resources readiness: %w", err)
				}
			case *OperationTrackExternalDependenciesReadiness:
				if err := e.tracker.TrackExternalDependenciesReadiness(ctx, resourcetracker.TrackExternalDependenciesReadinessOptions{FallbackNamespace: e.releaseNamespace.Name(), Timeout: e.trackTimeout}, op.Targets...); err != nil {
					return e.report, fmt.Errorf("error tracking external dependencies readiness: %w", err)
				}
			case *OperationTrackDeletion:
				if err := e.tracker.TrackDeletion(ctx, resourcetracker.TrackDeletionOptions{FallbackNamespace: e.releaseNamespace.Name(), Timeout: e.trackTimeout}, op.Targets...); err != nil {
					return e.report, fmt.Errorf("error tracking deletion: %w", err)
				}
			case *OperationCreateReleases:
				for _, rel := range op.Releases {
					if err := e.releaseStorage.Create(rel); err != nil {
						return e.report, fmt.Errorf("error creating release: %w", err)
					}
				}
			case *OperationUpdateReleases:
				for _, rel := range op.Releases {
					if err := e.releaseStorage.Update(rel); err != nil {
						return e.report, fmt.Errorf("error updating release: %w", err)
					}
				}
			default:
				panic("unknown operation")
			}
		}
	}

	return e.report, nil
}
