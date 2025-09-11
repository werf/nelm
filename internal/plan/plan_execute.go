package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/chanced/caps"
	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/pkg/log"
)

type ExecutePlanOptions struct {
	NetworkParallelism int
	ReadinessTimeout   time.Duration
	PresenceTimeout    time.Duration
	AbsenceTimeout     time.Duration
}

func ExecutePlan(
	parentCtx context.Context,
	plan *Plan,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	history release.Historier,
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	opts ExecutePlanOptions,
) error {
	ctx, ctxCancelFn := context.WithCancelCause(parentCtx)
	defer ctxCancelFn(fmt.Errorf("context canceled: plan execution finished"))

	opts.NetworkParallelism = lo.Max([]int{opts.NetworkParallelism, 1})

	workerPool := pool.New().WithContext(ctx).WithMaxGoroutines(opts.NetworkParallelism).WithCancelOnError().WithFirstError()
	completedOpsIDsCh := make(chan string, 100000)
	opsMap := plan.PredecessorMap()

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
			opID := opID
			delete(opsMap, opID)
			execOperation(
				opID,
				completedOpsIDsCh,
				workerPool,
				plan,
				taskStore,
				logStore,
				informerFactory,
				history,
				kubeClient,
				staticClient,
				dynamicClient,
				discoveryClient,
				mapper,
				ctxCancelFn,
				opts.ReadinessTimeout,
				opts.PresenceTimeout,
				opts.AbsenceTimeout,
			)
		}
	}

	log.Default.Debug(ctx, "Wait for all plan operations to complete")
	if err := workerPool.Wait(); err != nil {
		return fmt.Errorf("wait for operations completion: %w", err)
	}

	return nil
}

func execOperation(
	opID string,
	completedOpsIDsCh chan string,
	workerPool *pool.ContextPool,
	plan *Plan,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	history release.Historier,
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	ctxCancelFn context.CancelCauseFunc,
	readinessTimeout time.Duration,
	presenceTimeout time.Duration,
	absenceTimeout time.Duration,
) {
	workerPool.Go(func(ctx context.Context) error {
		var err error
		defer func() {
			if err != nil {
				ctxCancelFn(fmt.Errorf("context canceled: %w", err))
			}
		}()

		op := lo.Must(plan.Operation(opID))
		op.Status = operation.OperationStatusPending

		log.Default.Debug(ctx, caps.ToUpper(op.IDHuman()))
		err = execOp(
			ctx,
			op,
			taskStore,
			logStore,
			informerFactory,
			history,
			kubeClient,
			staticClient,
			dynamicClient,
			discoveryClient,
			mapper,
			readinessTimeout,
			presenceTimeout,
			absenceTimeout,
		)
		if err != nil {
			op.Status = operation.OperationStatusFailed
			return fmt.Errorf("execute operation: %w", err)
		}

		op.Status = operation.OperationStatusCompleted
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

func execOp(
	ctx context.Context,
	op *operation.Operation,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	history release.Historier,
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	readinessTimeout time.Duration,
	presenceTimeout time.Duration,
	absenceTimeout time.Duration,
) error {
	switch op.Type {
	case operation.OperationTypeCreate:
		return execOpCreate(ctx, op, kubeClient)
	case operation.OperationTypeRecreate:
		return execOpRecreate(ctx, op, taskStore, informerFactory, kubeClient, dynamicClient, mapper, absenceTimeout)
	case operation.OperationTypeUpdate:
		return execOpUpdate(ctx, op, kubeClient)
	case operation.OperationTypeApply:
		return execOpApply(ctx, op, kubeClient)
	case operation.OperationTypeDelete:
		return execOpDelete(ctx, op, kubeClient)
	case operation.OperationTypeTrackReadiness:
		return execOpTrackReadiness(ctx, op, taskStore, logStore, informerFactory, kubeClient, staticClient, dynamicClient, discoveryClient, mapper, readinessTimeout)
	case operation.OperationTypeTrackPresence:
		return execOpTrackPresence(ctx, op, taskStore, informerFactory, dynamicClient, mapper, presenceTimeout)
	case operation.OperationTypeTrackAbsence:
		return execOpTrackAbsence(ctx, op, taskStore, informerFactory, dynamicClient, mapper, absenceTimeout)
	case operation.OperationTypeCreateRelease:
		return execOpCreateRelease(ctx, op, history)
	case operation.OperationTypeUpdateRelease:
		return execOpUpdateRelease(ctx, op, history)
	case operation.OperationTypeDeleteRelease:
		return execOpDeleteRelease(ctx, op, history)
	case operation.OperationTypeNoop:
	default:
		panic("unexpected operation type")
	}

	return nil
}

func execOpCreate(ctx context.Context, op *operation.Operation, kubeClient kube.KubeClienter) error {
	opConfig := op.Config.(*operation.OperationConfigCreate)

	if _, err := kubeClient.Create(ctx, opConfig.ResourceSpec, kube.KubeClientCreateOptions{
		ForceReplicas: opConfig.ForceReplicas,
	}); err != nil {
		if errors.IsAlreadyExists(err) {
			if _, err := kubeClient.Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{}); err != nil {
				return fmt.Errorf("apply resource: %w", err)
			}
		} else {
			return fmt.Errorf("create resource: %w", err)
		}
	}

	return nil
}

func execOpRecreate(
	ctx context.Context,
	op *operation.Operation,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	kubeClient kube.KubeClienter,
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	absenceTimeout time.Duration,
) error {
	opConfig := op.Config.(*operation.OperationConfigRecreate)

	if err := kubeClient.Delete(ctx, opConfig.ResourceSpec.ResourceMeta, kube.KubeClientDeleteOptions{}); err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewAbsenceTaskState(opConfig.ResourceSpec.Name, opConfig.ResourceSpec.Namespace, opConfig.ResourceSpec.GroupVersionKind, statestore.AbsenceTaskStateOptions{}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddAbsenceTaskState(taskState)
	})

	tracker := dyntracker.NewDynamicAbsenceTracker(taskState, informerFactory, dynamicClient, mapper, dyntracker.DynamicAbsenceTrackerOptions{
		Timeout: absenceTimeout,
	})

	if err := tracker.Track(ctx); err != nil {
		return fmt.Errorf("track resource absence: %w", err)
	}

	if _, err := kubeClient.Create(ctx, opConfig.ResourceSpec, kube.KubeClientCreateOptions{
		ForceReplicas: opConfig.ForceReplicas,
	}); err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	return nil
}

func execOpUpdate(ctx context.Context, op *operation.Operation, kubeClient kube.KubeClienter) error {
	opConfig := op.Config.(*operation.OperationConfigUpdate)

	if _, err := kubeClient.Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{}); err != nil {
		return fmt.Errorf("apply resource: %w", err)
	}

	return nil
}

func execOpApply(ctx context.Context, op *operation.Operation, kubeClient kube.KubeClienter) error {
	opConfig := op.Config.(*operation.OperationConfigApply)

	if _, err := kubeClient.Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{}); err != nil {
		return fmt.Errorf("apply resource: %w", err)
	}

	return nil
}

func execOpDelete(ctx context.Context, op *operation.Operation, kubeClient kube.KubeClienter) error {
	opConfig := op.Config.(*operation.OperationConfigDelete)

	if err := kubeClient.Delete(ctx, opConfig.ResourceMeta, kube.KubeClientDeleteOptions{}); err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}

	return nil
}

func execOpTrackReadiness(
	ctx context.Context,
	op *operation.Operation,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	timeout time.Duration,
) error {
	opConfig := op.Config.(*operation.OperationConfigTrackReadiness)

	taskState := kdutil.NewConcurrent(
		statestore.NewReadinessTaskState(opConfig.ResourceMeta.Name, opConfig.ResourceMeta.Namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.ReadinessTaskStateOptions{
			FailMode:                opConfig.FailMode,
			TotalAllowFailuresCount: opConfig.FailuresAllowed,
		}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddReadinessTaskState(taskState)
	})

	tracker, err := dyntracker.NewDynamicReadinessTracker(ctx, taskState, logStore, informerFactory, staticClient, dynamicClient, discoveryClient, mapper, dyntracker.DynamicReadinessTrackerOptions{
		Timeout:                                  timeout,
		NoActivityTimeout:                        opConfig.NoActivityTimeout,
		IgnoreReadinessProbeFailsByContainerName: opConfig.IgnoreReadinessProbeFailsByContainerName,
		SaveLogsOnlyForNumberOfReplicas:          opConfig.SaveLogsOnlyForNumberOfReplicas,
		SaveLogsOnlyForContainers:                opConfig.SaveLogsOnlyForContainers,
		SaveLogsByRegex:                          opConfig.SaveLogsByRegex,
		SaveLogsByRegexForContainers:             opConfig.SaveLogsByRegexForContainers,
		IgnoreLogs:                               opConfig.IgnoreLogs,
		IgnoreLogsForContainers:                  opConfig.IgnoreLogsForContainers,
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

func execOpTrackPresence(
	ctx context.Context,
	op *operation.Operation,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	timeout time.Duration,
) error {
	opConfig := op.Config.(*operation.OperationConfigTrackPresence)

	taskState := kdutil.NewConcurrent(
		statestore.NewPresenceTaskState(opConfig.ResourceMeta.Name, opConfig.ResourceMeta.Namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.PresenceTaskStateOptions{}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddPresenceTaskState(taskState)
	})

	tracker := dyntracker.NewDynamicPresenceTracker(taskState, informerFactory, dynamicClient, mapper, dyntracker.DynamicPresenceTrackerOptions{
		Timeout: timeout,
	})

	if err := tracker.Track(ctx); err != nil {
		return fmt.Errorf("track resource presence: %w", err)
	}

	return nil
}

func execOpTrackAbsence(
	ctx context.Context,
	op *operation.Operation,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	timeout time.Duration,
) error {
	opConfig := op.Config.(*operation.OperationConfigTrackAbsence)

	taskState := kdutil.NewConcurrent(
		statestore.NewAbsenceTaskState(opConfig.ResourceMeta.Name, opConfig.ResourceMeta.Namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.AbsenceTaskStateOptions{}),
	)

	taskStore.RWTransaction(func(ts *statestore.TaskStore) {
		ts.AddAbsenceTaskState(taskState)
	})

	tracker := dyntracker.NewDynamicAbsenceTracker(taskState, informerFactory, dynamicClient, mapper, dyntracker.DynamicAbsenceTrackerOptions{
		Timeout: timeout,
	})

	if err := tracker.Track(ctx); err != nil {
		return fmt.Errorf("track resource absence: %w", err)
	}

	return nil
}

func execOpCreateRelease(
	ctx context.Context,
	op *operation.Operation,
	history release.Historier,
) error {
	opConfig := op.Config.(*operation.OperationConfigCreateRelease)

	if err := history.CreateRelease(ctx, opConfig.Release); err != nil {
		return fmt.Errorf("create release: %w", err)
	}

	return nil
}

func execOpUpdateRelease(
	ctx context.Context,
	op *operation.Operation,
	history release.Historier,
) error {
	opConfig := op.Config.(*operation.OperationConfigUpdateRelease)

	if err := history.UpdateRelease(ctx, opConfig.Release); err != nil {
		return fmt.Errorf("update release: %w", err)
	}

	return nil
}

func execOpDeleteRelease(
	ctx context.Context,
	op *operation.Operation,
	history release.Historier,
) error {
	opConfig := op.Config.(*operation.OperationConfigDeleteRelease)

	if err := history.DeleteRelease(ctx, opConfig.ReleaseName, opConfig.ReleaseRevision); err != nil {
		return fmt.Errorf("delete release: %w", err)
	}

	return nil
}
