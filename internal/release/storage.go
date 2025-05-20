package release

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"

	helmaction "github.com/werf/3p-helm/pkg/action"
	helmstorage "github.com/werf/3p-helm/pkg/storage"
	helmdriver "github.com/werf/3p-helm/pkg/storage/driver"
	"github.com/werf/nelm/pkg/log"
)

type ReleaseStorageOptions struct {
	HistoryLimit        int
	StaticClient        *kubernetes.Clientset
	SQLConnectionString string
}

func NewReleaseStorage(ctx context.Context, namespace, storageDriver string, opts ReleaseStorageOptions) (*helmstorage.Storage, error) {
	var storage *helmstorage.Storage

	lazyClient := helmaction.NewLazyClient(namespace, func() (*kubernetes.Clientset, error) {
		return opts.StaticClient, nil
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
