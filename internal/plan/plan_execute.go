package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"

	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type ExecutePlanOptions struct {
	common.TrackingOptions

	LegacyProgressReporter *LegacyProgressReporter
	NetworkParallelism     int
}

// Executes the given plan. It doesn't care what kind of plan it is (install, upgrade, failure plan,
// etc.). All the differences between these plans must be figured out earlier, e.g. in BuildPlan.
// This generic design must be preserved. Keep it simple: if something can be done on earlier
// stages, do it there.
func ExecutePlan(parentCtx context.Context, releaseNamespace string, plan *Plan, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], history release.Historier, clientFactory kube.ClientFactorier, opts ExecutePlanOptions) error {
	ctx, ctxCancelFn := context.WithCancelCause(parentCtx)
	defer ctxCancelFn(fmt.Errorf("context canceled: plan execution finished"))

	opts.NetworkParallelism = lo.Max([]int{opts.NetworkParallelism, 1})

	if opts.LegacyProgressReporter != nil {
		resolvedNS := buildResolvedNamespaces(plan, releaseNamespace, clientFactory.Mapper())
		opts.LegacyProgressReporter.startStage(plan, resolvedNS)
	}

	workerPool := pool.New().WithContext(ctx).WithMaxGoroutines(opts.NetworkParallelism).WithCancelOnError().WithFirstError()
	completedOpsIDsCh := make(chan string, 100000)
	opsMap := lo.Must(plan.Graph.PredecessorMap())

	log.Default.Debug(ctx, "Start plan operations")

	for i := 0; len(opsMap) > 0; i++ {
		if i > 0 {
			if ctx.Err() != nil {
				log.Default.Debug(ctx, "Stop scheduling plan operations due to context canceled: %s", context.Cause(ctx))

				break
			}

			var gotCompletedOpID bool
			for len(completedOpsIDsCh) > 0 {
				completedOpID := <-completedOpsIDsCh
				gotCompletedOpID = true

				for _, edgeMap := range opsMap {
					delete(edgeMap, completedOpID)
				}
			}

			if !gotCompletedOpID {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		executableOpsIDs := findExecutableOpsIDs(opsMap)
		for _, opID := range executableOpsIDs {
			delete(opsMap, opID)
			execOperation(opID, releaseNamespace, completedOpsIDsCh, workerPool, plan, taskStore, logStore, informerFactory, history, clientFactory, ctxCancelFn, opts.TrackReadinessTimeout, opts.TrackCreationTimeout, opts.TrackDeletionTimeout, opts.LegacyProgressReporter)
		}
	}

	log.Default.Debug(ctx, "Wait for all plan operations to complete")

	if err := workerPool.Wait(); err != nil {
		return fmt.Errorf("wait for operations completion: %w", err)
	}

	if ctx.Err() != nil {
		return fmt.Errorf("execution canceled: %w", context.Cause(ctx))
	}

	return nil
}

func execOperation(opID, releaseNamespace string, completedOpsIDsCh chan string, workerPool *pool.ContextPool, plan *Plan, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], history release.Historier, clientFactory kube.ClientFactorier, ctxCancelFn context.CancelCauseFunc, readinessTimeout, presenceTimeout, absenceTimeout time.Duration, reporter *LegacyProgressReporter) {
	workerPool.Go(func(ctx context.Context) error {
		var err error
		defer func() {
			if err != nil {
				ctxCancelFn(fmt.Errorf("context canceled: %w", err))
			}
		}()

		op := lo.Must(plan.Operation(opID))
		reportOperationStatus(op, OperationStatusPending, reporter)

		log.Default.Debug(ctx, util.Capitalize(op.IDHuman()))

		if err = execOp(ctx, op, releaseNamespace, taskStore, logStore, informerFactory, history, clientFactory, readinessTimeout, presenceTimeout, absenceTimeout); err != nil {
			reportOperationStatus(op, OperationStatusFailed, reporter)
			return fmt.Errorf("execute operation: %w", err)
		}

		reportOperationStatus(op, OperationStatusCompleted, reporter)

		completedOpsIDsCh <- opID

		return nil
	})
}

func findExecutableOpsIDs(opsMap map[string]map[string]graph.Edge[string]) []string {
	var executableOpsIDs []string
	for opID, edgeMap := range opsMap {
		if len(edgeMap) == 0 {
			executableOpsIDs = append(executableOpsIDs, opID)
		}
	}

	return executableOpsIDs
}

func execOp(ctx context.Context, op *Operation, releaseNamespace string, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], history release.Historier, clientFactory kube.ClientFactorier, readinessTimeout, presenceTimeout, absenceTimeout time.Duration) error {
	switch op.Type {
	case OperationTypeCreate:
		return execOpCreate(ctx, op, releaseNamespace, clientFactory)
	case OperationTypeRecreate:
		return execOpRecreate(ctx, op, releaseNamespace, taskStore, informerFactory, absenceTimeout, clientFactory)
	case OperationTypeUpdate:
		return execOpUpdate(ctx, op, releaseNamespace, clientFactory)
	case OperationTypeApply:
		return execOpApply(ctx, op, releaseNamespace, clientFactory)
	case OperationTypeDelete:
		return execOpDelete(ctx, op, releaseNamespace, clientFactory)
	case OperationTypeTrackReadiness:
		return execOpTrackReadiness(ctx, op, releaseNamespace, taskStore, logStore, informerFactory, readinessTimeout, clientFactory)
	case OperationTypeTrackPresence:
		return execOpTrackPresence(ctx, op, releaseNamespace, taskStore, informerFactory, presenceTimeout, clientFactory)
	case OperationTypeTrackAbsence:
		return execOpTrackAbsence(ctx, op, releaseNamespace, taskStore, informerFactory, absenceTimeout, clientFactory)
	case OperationTypeCreateRelease:
		return execOpCreateRelease(ctx, op, history)
	case OperationTypeUpdateRelease:
		return execOpUpdateRelease(ctx, op, history)
	case OperationTypeDeleteRelease:
		return execOpDeleteRelease(ctx, op, history)
	case OperationTypeNoop:
	default:
		panic("unexpected operation type")
	}

	return nil
}

func execOpCreate(ctx context.Context, op *Operation, releaseNamespace string, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigCreate)

	if _, err := clientFactory.KubeClient().Create(ctx, opConfig.ResourceSpec, kube.KubeClientCreateOptions{
		DefaultNamespace: releaseNamespace,
		ForceReplicas:    opConfig.ForceReplicas,
	}); err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	return nil
}

func execOpRecreate(ctx context.Context, op *Operation, releaseNamespace string, taskStore *kdutil.Concurrent[*statestore.TaskStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], absenceTimeout time.Duration, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigRecreate)

	if err := clientFactory.KubeClient().Delete(ctx, opConfig.ResourceSpec.ResourceMeta, kube.KubeClientDeleteOptions{
		DefaultNamespace:  releaseNamespace,
		PropagationPolicy: opConfig.DeletePropagation,
	}); err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}

	namespace, err := getNamespace(opConfig.ResourceSpec.ResourceMeta, releaseNamespace, clientFactory)
	if err != nil {
		return fmt.Errorf("determine resource namespace: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewAbsenceTaskState(opConfig.ResourceSpec.Name, namespace, opConfig.ResourceSpec.GroupVersionKind, statestore.AbsenceTaskStateOptions{}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddAbsenceTaskState(taskState)
	})

	tracker := dyntracker.NewDynamicAbsenceTracker(taskState, informerFactory, clientFactory.Dynamic(), clientFactory.Mapper(), dyntracker.DynamicAbsenceTrackerOptions{
		Timeout: absenceTimeout,
	})

	if err := tracker.Track(ctx); err != nil {
		return fmt.Errorf("track resource absence: %w", err)
	}

	if _, err := clientFactory.KubeClient().Create(ctx, opConfig.ResourceSpec, kube.KubeClientCreateOptions{
		DefaultNamespace: releaseNamespace,
		ForceReplicas:    opConfig.ForceReplicas,
	}); err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	return nil
}

func execOpUpdate(ctx context.Context, op *Operation, releaseNamespace string, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigUpdate)

	if _, err := clientFactory.KubeClient().Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{
		DefaultNamespace: releaseNamespace,
	}); err != nil {
		return fmt.Errorf("apply resource: %w", err)
	}

	return nil
}

func execOpApply(ctx context.Context, op *Operation, releaseNamespace string, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigApply)

	if _, err := clientFactory.KubeClient().Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{
		DefaultNamespace: releaseNamespace,
	}); err != nil {
		return fmt.Errorf("apply resource: %w", err)
	}

	return nil
}

func execOpDelete(ctx context.Context, op *Operation, releaseNamespace string, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigDelete)

	if err := clientFactory.KubeClient().Delete(ctx, opConfig.ResourceMeta, kube.KubeClientDeleteOptions{
		DefaultNamespace:  releaseNamespace,
		PropagationPolicy: opConfig.DeletePropagation,
	}); err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}

	return nil
}

func execOpTrackReadiness(ctx context.Context, op *Operation, releaseNamespace string, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], timeout time.Duration, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigTrackReadiness)

	namespace, err := getNamespace(opConfig.ResourceMeta, releaseNamespace, clientFactory)
	if err != nil {
		return fmt.Errorf("determine resource namespace: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewReadinessTaskState(opConfig.ResourceMeta.Name, namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.ReadinessTaskStateOptions{
			FailMode:                opConfig.FailMode,
			TotalAllowFailuresCount: opConfig.FailuresAllowed,
		}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddReadinessTaskState(taskState)
	})

	tracker, err := dyntracker.NewDynamicReadinessTracker(ctx, taskState, logStore, informerFactory, clientFactory.Static(), clientFactory.Dynamic(), clientFactory.Discovery(), clientFactory.Mapper(), dyntracker.DynamicReadinessTrackerOptions{
		Timeout:                                  timeout,
		NoActivityTimeout:                        opConfig.NoActivityTimeout,
		IgnoreReadinessProbeFailsByContainerName: opConfig.IgnoreReadinessProbeFailsByContainerName,
		SaveLogsOnlyForNumberOfReplicas:          opConfig.SaveLogsOnlyForNumberOfReplicas,
		SaveLogsOnlyForContainers:                opConfig.SaveLogsOnlyForContainers,
		SaveLogsByRegex:                          opConfig.SaveLogsByRegex,
		SaveLogsByRegexForContainers:             opConfig.SaveLogsByRegexForContainers,
		IgnoreLogs:                               opConfig.IgnoreLogs,
		IgnoreLogsForContainers:                  opConfig.IgnoreLogsForContainers,
		IgnoreLogsByRegex:                        opConfig.IgnoreLogsByRegex,
		IgnoreLogsByRegexForContainers:           opConfig.IgnoreLogsByRegexForContainers,
		SaveEvents:                               opConfig.SaveEvents,
	})
	if err != nil {
		return fmt.Errorf("construct dynamic readiness tracker: %w", err)
	}

	if err := tracker.Track(ctx); err != nil {
		return fmt.Errorf("track resource readiness: %w", err)
	}

	return nil
}

func execOpTrackPresence(ctx context.Context, op *Operation, releaseNamespace string, taskStore *kdutil.Concurrent[*statestore.TaskStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], timeout time.Duration, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigTrackPresence)

	namespace, err := getNamespace(opConfig.ResourceMeta, releaseNamespace, clientFactory)
	if err != nil {
		return fmt.Errorf("determine resource namespace: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewPresenceTaskState(opConfig.ResourceMeta.Name, namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.PresenceTaskStateOptions{}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddPresenceTaskState(taskState)
	})

	tracker := dyntracker.NewDynamicPresenceTracker(taskState, informerFactory, clientFactory.Dynamic(), clientFactory.Mapper(), dyntracker.DynamicPresenceTrackerOptions{
		Timeout: timeout,
	})

	if err := tracker.Track(ctx); err != nil {
		return fmt.Errorf("track resource presence: %w", err)
	}

	return nil
}

func execOpTrackAbsence(ctx context.Context, op *Operation, releaseNamespace string, taskStore *kdutil.Concurrent[*statestore.TaskStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], timeout time.Duration, clientFactory kube.ClientFactorier) error {
	opConfig := op.Config.(*OperationConfigTrackAbsence)

	namespace, err := getNamespace(opConfig.ResourceMeta, releaseNamespace, clientFactory)
	if err != nil {
		return fmt.Errorf("determine resource namespace: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewAbsenceTaskState(opConfig.ResourceMeta.Name, namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.AbsenceTaskStateOptions{}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddAbsenceTaskState(taskState)
	})

	tracker := dyntracker.NewDynamicAbsenceTracker(taskState, informerFactory, clientFactory.Dynamic(), clientFactory.Mapper(), dyntracker.DynamicAbsenceTrackerOptions{
		Timeout: timeout,
	})

	if err := tracker.Track(ctx); err != nil {
		return fmt.Errorf("track resource absence: %w", err)
	}

	return nil
}

func execOpCreateRelease(ctx context.Context, op *Operation, history release.Historier) error {
	opConfig := op.Config.(*OperationConfigCreateRelease)

	if err := history.CreateRelease(ctx, opConfig.Release); err != nil {
		return fmt.Errorf("create release: %w", err)
	}

	return nil
}

func execOpUpdateRelease(ctx context.Context, op *Operation, history release.Historier) error {
	opConfig := op.Config.(*OperationConfigUpdateRelease)

	if err := history.UpdateRelease(ctx, opConfig.Release); err != nil {
		return fmt.Errorf("update release: %w", err)
	}

	return nil
}

func execOpDeleteRelease(ctx context.Context, op *Operation, history release.Historier) error {
	opConfig := op.Config.(*OperationConfigDeleteRelease)

	if err := history.DeleteRelease(ctx, opConfig.ReleaseName, opConfig.ReleaseRevision); err != nil {
		return fmt.Errorf("delete release: %w", err)
	}

	return nil
}

func getNamespace(resMeta *spec.ResourceMeta, releaseNamespace string, clientFactory kube.ClientFactorier) (string, error) {
	var namespace string
	if namespaced, err := spec.Namespaced(resMeta.GroupVersionKind, clientFactory.Mapper()); err != nil {
		return "", fmt.Errorf("check if resource is namespaced: %w", err)
	} else if namespaced {
		if resMeta.Namespace != "" {
			namespace = resMeta.Namespace
		} else {
			namespace = releaseNamespace
		}
	}

	return namespace, nil
}
