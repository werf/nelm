package lock

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/common-go/pkg/locker_with_retry"
	"github.com/werf/lockgate"
	"github.com/werf/lockgate/pkg/distributed_locker"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/log"
)

// NOTE: LockManager for not is not multithreaded due to the lack of support of contexts in the lockgate library
type LockManager struct {
	Namespace       string
	LockerWithRetry *locker_with_retry.LockerWithRetry
}

type ConfigMapLocker struct {
	ConfigMapName, Namespace string

	Locker lockgate.Locker

	clientFactory    kube.ClientFactorier
	releaseNamespace string
	createNamespace  bool
}

type ConfigMapLockerOptions struct {
	CreateNamespace bool
}

func NewConfigMapLocker(
	configMapName, namespace, releaseNamespace string,
	locker lockgate.Locker,
	clientFactory kube.ClientFactorier,
	options ConfigMapLockerOptions,
) *ConfigMapLocker {
	return &ConfigMapLocker{
		ConfigMapName:    configMapName,
		Namespace:        namespace,
		Locker:           locker,
		clientFactory:    clientFactory,
		createNamespace:  options.CreateNamespace,
		releaseNamespace: releaseNamespace,
	}
}

func (locker *ConfigMapLocker) Acquire(lockName string, opts lockgate.AcquireOptions) (
	bool,
	lockgate.LockHandle,
	error,
) {
	if err := getOrCreateConfigMapWithNamespaceIfNotExists(locker.clientFactory, locker.Namespace, locker.releaseNamespace, locker.ConfigMapName, locker.createNamespace); err != nil {
		return false, lockgate.LockHandle{}, fmt.Errorf("unable to prepare kubernetes cm/%s in ns/%s: %w", locker.ConfigMapName, locker.Namespace, err)
	}

	return locker.Locker.Acquire(lockName, opts)
}

func (locker *ConfigMapLocker) Release(lock lockgate.LockHandle) error {
	return locker.Locker.Release(lock)
}

func NewLockManager(ctx context.Context, namespace string, createNamespace bool, clientFactory kube.ClientFactorier) (*LockManager, error) {
	configMapName := "werf-synchronization"

	locker := distributed_locker.NewKubernetesLocker(
		clientFactory.Dynamic(), schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "configmaps",
		}, configMapName, namespace,
	)
	cmLocker := NewConfigMapLocker(configMapName, namespace, namespace, locker, clientFactory, ConfigMapLockerOptions{CreateNamespace: createNamespace})
	lockerWithRetry := locker_with_retry.NewLockerWithRetry(ctx, cmLocker, locker_with_retry.LockerWithRetryOptions{
		MaxAcquireAttempts: 10,
		MaxReleaseAttempts: 10,
		CustomLogWarnFunc: func(msg string) {
			log.Default.Warn(ctx, msg)
		},
		CustomLogErrFunc: func(msg string) {
			log.Default.Error(ctx, msg)
		},
	})

	return &LockManager{
		Namespace:       namespace,
		LockerWithRetry: lockerWithRetry,
	}, nil
}

func (lockManager *LockManager) LockRelease(
	ctx context.Context,
	releaseName string,
) (lockgate.LockHandle, error) {
	// TODO: add support of context into lockgate
	lockManager.LockerWithRetry.Ctx = ctx
	_, handle, err := lockManager.LockerWithRetry.Acquire(fmt.Sprintf("release/%s", releaseName), setupLockerDefaultOptions(ctx, lockgate.AcquireOptions{}))

	return handle, err
}

func (lockManager *LockManager) Unlock(handle lockgate.LockHandle) error {
	defer func() {
		lockManager.LockerWithRetry.Ctx = nil
	}()

	return lockManager.LockerWithRetry.Release(handle)
}

func setupLockerDefaultOptions(
	ctx context.Context,
	opts lockgate.AcquireOptions,
) lockgate.AcquireOptions {
	if opts.OnWaitFunc == nil {
		opts.OnWaitFunc = defaultLockerOnWait(ctx)
	}

	if opts.OnLostLeaseFunc == nil {
		opts.OnLostLeaseFunc = defaultLockerOnLostLease
	}

	return opts
}

func defaultLockerOnWait(ctx context.Context) func(lockName string, doWait func() error) error {
	return func(lockName string, doWait func() error) error {
		log.Default.Info(ctx, fmt.Sprintf("Waiting for locked %q", lockName))
		return doWait()
	}
}

func defaultLockerOnLostLease(lock lockgate.LockHandle) error {
	return fmt.Errorf("locker has lost the lease for lock %q uuid %q. The process will stop immediately.\nPossible reasons:\n- Connection issues with Kubernetes API.\n- Network delays caused lease renewal requests to fail", lock.LockName, lock.UUID)
}

func createNamespaceIfNotExists(clientFactory kube.ClientFactorier, namespace, releaseNamespace string) error {
	unstruct := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": namespace,
			},
		},
	}

	resSpec := spec.NewResourceSpec(unstruct, releaseNamespace, spec.ResourceSpecOptions{})

	if _, err := clientFactory.KubeClient().Get(context.Background(), resSpec.ResourceMeta, kube.KubeClientGetOptions{
		DefaultNamespace: releaseNamespace,
		TryCache:         true,
	}); err != nil {
		if kube.IsNotFoundErr(err) {
			if _, err := clientFactory.KubeClient().Create(context.Background(), resSpec, kube.KubeClientCreateOptions{
				DefaultNamespace: releaseNamespace,
			}); err != nil {
				return fmt.Errorf("create namespace %q: %w", namespace, err)
			}
		}

		return fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	return nil
}

func getOrCreateConfigMapWithNamespaceIfNotExists(clientFactory kube.ClientFactorier, namespace, releaseNamespace, configMapName string, createNamespace bool) error {
	unstruct := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": configMapName,
			},
		},
	}

	resSpec := spec.NewResourceSpec(unstruct, releaseNamespace, spec.ResourceSpecOptions{})

	if _, err := clientFactory.KubeClient().Get(context.Background(), resSpec.ResourceMeta, kube.KubeClientGetOptions{
		DefaultNamespace: releaseNamespace,
		TryCache:         true,
	}); err != nil {
		if kube.IsNotFoundErr(err) {
			if createNamespace {
				if err := createNamespaceIfNotExists(clientFactory, namespace, releaseNamespace); err != nil {
					return fmt.Errorf("create namespace if not exists: %w", err)
				}
			}

			if _, err := clientFactory.KubeClient().Create(context.Background(), resSpec, kube.KubeClientCreateOptions{
				DefaultNamespace: releaseNamespace,
			}); err != nil {
				return fmt.Errorf("create resource: %w", err)
			}

			return nil
		}

		return fmt.Errorf("get resource: %w", err)
	}

	return nil
}
