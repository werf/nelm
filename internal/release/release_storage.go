package release

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"

	helmaction "github.com/werf/3p-helm/pkg/action"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	helmstorage "github.com/werf/3p-helm/pkg/storage"
	helmdriver "github.com/werf/3p-helm/pkg/storage/driver"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/pkg/log"
)

var _ ReleaseStorager = (*helmstorage.Storage)(nil)

type ReleaseStorager interface {
	Create(rls *helmrelease.Release) error
	Update(rls *helmrelease.Release) error
	Delete(name string, version int) (*helmrelease.Release, error)
	Query(labels map[string]string) ([]*helmrelease.Release, error)
}

type ReleaseStorageOptions struct {
	HistoryLimit        int
	SQLConnectionString string
}

func NewReleaseStorage(ctx context.Context, namespace, storageDriver string, clientFactory kube.ClientFactorier, opts ReleaseStorageOptions) (*helmstorage.Storage, error) {
	var storage *helmstorage.Storage

	lazyClient := helmaction.NewLazyClient(namespace, func() (*kubernetes.Clientset, error) {
		return clientFactory.Static().(*kubernetes.Clientset), nil
	})

	logFn := func(format string, a ...interface{}) {
		log.Default.Debug(ctx, format, a...)
	}

	switch storageDriver {
	case "secret", "secrets", "":
		driver := helmdriver.NewSecrets(helmaction.NewSecretClient(lazyClient))
		driver.Log = logFn

		storage = helmstorage.Init(driver)
	case "configmap", "configmaps":
		driver := helmdriver.NewConfigMaps(helmaction.NewConfigMapClient(lazyClient))
		driver.Log = logFn

		storage = helmstorage.Init(driver)
	case "memory":
		driver := helmdriver.NewMemory()
		driver.SetNamespace(namespace)

		storage = helmstorage.Init(driver)
	case "sql":
		driver, err := helmdriver.NewSQL(opts.SQLConnectionString, logFn, namespace)
		if err != nil {
			return nil, fmt.Errorf("construct sql driver: %w", err)
		}

		storage = helmstorage.Init(driver)
	default:
		panic(fmt.Sprintf("Unknown storage driver: %s", storageDriver))
	}

	storage.MaxHistory = opts.HistoryLimit

	return storage, nil
}
