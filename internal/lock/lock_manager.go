package lock

import (
	"context"
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/common-go/pkg/locker_with_retry"
	kdkube "github.com/werf/kubedog/pkg/kube"
	"github.com/werf/lockgate"
	"github.com/werf/lockgate/pkg/distributed_locker"
	"github.com/werf/logboek"
)

// NOTE: LockManager for not is not multithreaded due to the lack of support of contexts in the lockgate library
type LockManager struct {
	Namespace       string
	LockerWithRetry *locker_with_retry.LockerWithRetry
}

type ConfigMapLocker struct {
	ConfigMapName, Namespace string

	Locker lockgate.Locker

	kubeClient      kubernetes.Interface
	createNamespace bool
}

type ConfigMapLockerOptions struct {
	CreateNamespace bool
	KubeClient      kubernetes.Interface
}

func NewConfigMapLocker(
	configMapName, namespace string,
	locker lockgate.Locker,
	options ConfigMapLockerOptions,
) *ConfigMapLocker {
	var kubeClient kubernetes.Interface
	if options.KubeClient != nil {
		kubeClient = options.KubeClient
	} else {
		kubeClient = kdkube.Client
	}

	return &ConfigMapLocker{
		ConfigMapName:   configMapName,
		Namespace:       namespace,
		Locker:          locker,
		kubeClient:      kubeClient,
		createNamespace: options.CreateNamespace,
	}
}

func (locker *ConfigMapLocker) Acquire(lockName string, opts lockgate.AcquireOptions) (
	bool,
	lockgate.LockHandle,
	error,
) {
	if _, err := getOrCreateConfigMapWithNamespaceIfNotExists(locker.kubeClient, locker.Namespace, locker.ConfigMapName, locker.createNamespace); err != nil {
		return false, lockgate.LockHandle{}, fmt.Errorf("unable to prepare kubernetes cm/%s in ns/%s: %w", locker.ConfigMapName, locker.Namespace, err)
	}

	return locker.Locker.Acquire(lockName, opts)
}

func (locker *ConfigMapLocker) Release(lock lockgate.LockHandle) error {
	return locker.Locker.Release(lock)
}

func NewLockManager(
	namespace string,
	createNamespace bool,
	kubeClient kubernetes.Interface,
	dynamicKubeClient dynamic.Interface,
) (*LockManager, error) {
	configMapName := "werf-synchronization"

	var dynKubeClient dynamic.Interface
	if dynamicKubeClient != nil {
		dynKubeClient = dynamicKubeClient
	} else {
		dynKubeClient = kdkube.DynamicClient
	}

	locker := distributed_locker.NewKubernetesLocker(
		dynKubeClient, schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "configmaps",
		}, configMapName, namespace,
	)
	cmLocker := NewConfigMapLocker(configMapName, namespace, locker, ConfigMapLockerOptions{CreateNamespace: createNamespace, KubeClient: kubeClient})
	lockerWithRetry := locker_with_retry.NewLockerWithRetry(context.Background(), cmLocker, locker_with_retry.LockerWithRetryOptions{MaxAcquireAttempts: 10, MaxReleaseAttempts: 10})

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
		logProcessMsg := fmt.Sprintf("Waiting for locked %q", lockName)
		return logboek.Context(ctx).Info().LogProcessInline(logProcessMsg).DoError(doWait)
	}
}

func defaultLockerOnLostLease(lock lockgate.LockHandle) error {
	return fmt.Errorf("locker has lost the lease for lock %q uuid %q. The process will stop immediately.\nPossible reasons:\n- Connection issues with Kubernetes API.\n- Network delays caused lease renewal requests to fail.", lock.LockName, lock.UUID)
}

func createNamespaceIfNotExists(client kubernetes.Interface, namespace string) error {
	if _, err := client.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{}); errors.IsNotFound(err) {
		ns := &v1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		if _, err := client.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{}); errors.IsAlreadyExists(err) {
			return nil
		} else if err != nil {
			return fmt.Errorf("create Namespace %s error: %w", namespace, err)
		}
	} else if err != nil {
		return fmt.Errorf("get Namespace %s error: %w", namespace, err)
	}
	return nil
}

func getOrCreateConfigMapWithNamespaceIfNotExists(
	client kubernetes.Interface,
	namespace, configMapName string,
	createNamespace bool,
) (*v1.ConfigMap, error) {
	obj, err := client.CoreV1().ConfigMaps(namespace).Get(context.Background(), configMapName, metav1.GetOptions{})
	switch {
	case errors.IsNotFound(err):
		if createNamespace {
			if err := createNamespaceIfNotExists(client, namespace); err != nil {
				return nil, err
			}
		}

		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: configMapName},
		}

		obj, err := client.CoreV1().ConfigMaps(namespace).Create(context.Background(), cm, metav1.CreateOptions{})
		switch {
		case errors.IsAlreadyExists(err):
			obj, err := client.CoreV1().ConfigMaps(namespace).Get(context.Background(), configMapName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("get ConfigMap %s error: %w", configMapName, err)
			}

			return obj, nil
		case err != nil:
			return nil, fmt.Errorf("create ConfigMap %s error: %w", cm.Name, err)
		default:
			return obj, nil
		}
	case err != nil:
		return nil, fmt.Errorf("get ConfigMap %s error: %w", configMapName, err)
	default:
		return obj, nil
	}
}
