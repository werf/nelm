package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/meta"
	"github.com/werf/nelm/internal/util"
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
	releaseNamespace string,
	plan *Plan,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	history release.Historier,
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper apimeta.ResettableRESTMapper,
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
				releaseNamespace,
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
	releaseNamespace string,
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
	mapper apimeta.ResettableRESTMapper,
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
		op.Status = OperationStatusPending

		log.Default.Debug(ctx, util.Capitalize(op.IDHuman()))
		err = execOp(
			ctx,
			op,
			releaseNamespace,
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
			op.Status = OperationStatusFailed
			return fmt.Errorf("execute operation: %w", err)
		}

		op.Status = OperationStatusCompleted
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
	op *Operation,
	releaseNamespace string,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	history release.Historier,
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper apimeta.ResettableRESTMapper,
	readinessTimeout time.Duration,
	presenceTimeout time.Duration,
	absenceTimeout time.Duration,
) error {
	switch op.Type {
	case OperationTypeCreate:
		return execOpCreate(ctx, op, kubeClient, releaseNamespace)
	case OperationTypeRecreate:
		return execOpRecreate(ctx, op, releaseNamespace, taskStore, informerFactory, kubeClient, dynamicClient, mapper, absenceTimeout)
	case OperationTypeUpdate:
		return execOpUpdate(ctx, op, kubeClient, releaseNamespace)
	case OperationTypeApply:
		return execOpApply(ctx, op, kubeClient, releaseNamespace)
	case OperationTypeDelete:
		return execOpDelete(ctx, op, kubeClient, releaseNamespace)
	case OperationTypeTrackReadiness:
		return execOpTrackReadiness(ctx, op, releaseNamespace, taskStore, logStore, informerFactory, kubeClient, staticClient, dynamicClient, discoveryClient, mapper, readinessTimeout)
	case OperationTypeTrackPresence:
		return execOpTrackPresence(ctx, op, releaseNamespace, taskStore, informerFactory, dynamicClient, mapper, presenceTimeout)
	case OperationTypeTrackAbsence:
		return execOpTrackAbsence(ctx, op, releaseNamespace, taskStore, informerFactory, dynamicClient, mapper, absenceTimeout)
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

func execOpCreate(ctx context.Context, op *Operation, kubeClient kube.KubeClienter, releaseNamespace string) error {
	opConfig := op.Config.(*OperationConfigCreate)

	if _, err := kubeClient.Create(ctx, opConfig.ResourceSpec, kube.KubeClientCreateOptions{
		DefaultNamespace: releaseNamespace,
		ForceReplicas:    opConfig.ForceReplicas,
	}); err != nil {
		if errors.IsAlreadyExists(err) {
			if _, err := kubeClient.Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{
				DefaultNamespace: releaseNamespace,
			}); err != nil {
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
	op *Operation,
	releaseNamespace string,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	kubeClient kube.KubeClienter,
	dynamicClient dynamic.Interface,
	mapper apimeta.ResettableRESTMapper,
	absenceTimeout time.Duration,
) error {
	opConfig := op.Config.(*OperationConfigRecreate)

	if err := kubeClient.Delete(ctx, opConfig.ResourceSpec.ResourceMeta, kube.KubeClientDeleteOptions{
		DefaultNamespace: releaseNamespace,
	}); err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}

	namespace, err := getNamespace(opConfig.ResourceSpec.ResourceMeta, mapper, releaseNamespace)
	if err != nil {
		return fmt.Errorf("determine resource namespace: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewAbsenceTaskState(opConfig.ResourceSpec.Name, namespace, opConfig.ResourceSpec.GroupVersionKind, statestore.AbsenceTaskStateOptions{}),
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
		DefaultNamespace: releaseNamespace,
		ForceReplicas:    opConfig.ForceReplicas,
	}); err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	return nil
}

func execOpUpdate(ctx context.Context, op *Operation, kubeClient kube.KubeClienter, releaseNamespace string) error {
	opConfig := op.Config.(*OperationConfigUpdate)

	if _, err := kubeClient.Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{
		DefaultNamespace: releaseNamespace,
	}); err != nil {
		return fmt.Errorf("apply resource: %w", err)
	}

	return nil
}

func execOpApply(ctx context.Context, op *Operation, kubeClient kube.KubeClienter, releaseNamespace string) error {
	opConfig := op.Config.(*OperationConfigApply)

	if _, err := kubeClient.Apply(ctx, opConfig.ResourceSpec, kube.KubeClientApplyOptions{
		DefaultNamespace: releaseNamespace,
	}); err != nil {
		return fmt.Errorf("apply resource: %w", err)
	}

	return nil
}

func execOpDelete(ctx context.Context, op *Operation, kubeClient kube.KubeClienter, releaseNamespace string) error {
	opConfig := op.Config.(*OperationConfigDelete)

	if err := kubeClient.Delete(ctx, opConfig.ResourceMeta, kube.KubeClientDeleteOptions{
		DefaultNamespace: releaseNamespace,
	}); err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}

	return nil
}

func execOpTrackReadiness(
	ctx context.Context,
	op *Operation,
	releaseNamespace string,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper apimeta.ResettableRESTMapper,
	timeout time.Duration,
) error {
	opConfig := op.Config.(*OperationConfigTrackReadiness)

	namespace, err := getNamespace(opConfig.ResourceMeta, mapper, releaseNamespace)
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
	op *Operation,
	releaseNamespace string,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	dynamicClient dynamic.Interface,
	mapper apimeta.ResettableRESTMapper,
	timeout time.Duration,
) error {
	opConfig := op.Config.(*OperationConfigTrackPresence)

	namespace, err := getNamespace(opConfig.ResourceMeta, mapper, releaseNamespace)
	if err != nil {
		return fmt.Errorf("determine resource namespace: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewPresenceTaskState(opConfig.ResourceMeta.Name, namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.PresenceTaskStateOptions{}),
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
	op *Operation,
	releaseNamespace string,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	dynamicClient dynamic.Interface,
	mapper apimeta.ResettableRESTMapper,
	timeout time.Duration,
) error {
	opConfig := op.Config.(*OperationConfigTrackAbsence)

	namespace, err := getNamespace(opConfig.ResourceMeta, mapper, releaseNamespace)
	if err != nil {
		return fmt.Errorf("determine resource namespace: %w", err)
	}

	taskState := kdutil.NewConcurrent(
		statestore.NewAbsenceTaskState(opConfig.ResourceMeta.Name, namespace, opConfig.ResourceMeta.GroupVersionKind, statestore.AbsenceTaskStateOptions{}),
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
	op *Operation,
	history release.Historier,
) error {
	opConfig := op.Config.(*OperationConfigCreateRelease)

	if err := history.CreateRelease(ctx, opConfig.Release); err != nil {
		return fmt.Errorf("create release: %w", err)
	}

	return nil
}

func execOpUpdateRelease(
	ctx context.Context,
	op *Operation,
	history release.Historier,
) error {
	opConfig := op.Config.(*OperationConfigUpdateRelease)

	if err := history.UpdateRelease(ctx, opConfig.Release); err != nil {
		return fmt.Errorf("update release: %w", err)
	}

	return nil
}

func execOpDeleteRelease(
	ctx context.Context,
	op *Operation,
	history release.Historier,
) error {
	opConfig := op.Config.(*OperationConfigDeleteRelease)

	if err := history.DeleteRelease(ctx, opConfig.ReleaseName, opConfig.ReleaseRevision); err != nil {
		return fmt.Errorf("delete release: %w", err)
	}

	return nil
}

func getNamespace(resMeta *meta.ResourceMeta, mapper apimeta.ResettableRESTMapper, releaseNamespace string) (string, error) {
	var namespace string
	if namespaced, err := resource.Namespaced(resMeta.GroupVersionKind, mapper); err != nil {
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
