package release

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"

	helmaction "github.com/werf/nelm/pkg/helm/pkg/action"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	helmdriver "github.com/werf/nelm/pkg/helm/pkg/storage/driver"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/log"
)

var _ ReleaseStorager = (*helmstorage.Storage)(nil)

// Minimal interface for Helm storage drivers.
type ReleaseStorager interface {
	Create(rls *helmrelease.Release) error
	Update(rls *helmrelease.Release) error
	Delete(name string, version int) (*helmrelease.Release, error)
	Query(labels map[string]string) ([]*helmrelease.Release, error)
	// GetRelease returns a single release revision. version == 0 means the latest revision.
	GetRelease(name string, version int) (*helmrelease.Release, error)
}

type ReleaseStorageOptions struct {
	HistoryLimit  int
	SQLConnection string
}

// Constructs Helm release storage driver.
func NewReleaseStorage(ctx context.Context, namespace, storageDriver string, clientFactory kube.ClientFactorier, opts ReleaseStorageOptions) (*helmstorage.Storage, error) {
	lazyClient := helmaction.NewLazyClient(namespace, func() (*kubernetes.Clientset, error) {
		return clientFactory.Static().(*kubernetes.Clientset), nil
	})

	logFn := func(format string, a ...interface{}) {
		log.Default.Debug(ctx, format, a...)
	}

	var storage *helmstorage.Storage

	switch storageDriver {
	case "secret", "secrets", "":
		metadataClient, err := metadata.NewForConfig(clientFactory.KubeConfig().RestConfig)
		if err != nil {
			return nil, fmt.Errorf("construct release metadata client: %w", err)
		}

		driver := helmdriver.NewSecrets(helmaction.NewSecretClient(lazyClient))
		driver.Log = logFn
		driver.MetadataClient = metadataClient
		driver.Namespace = namespace

		storage = helmstorage.Init(driver)
	case "configmap", "configmaps":
		metadataClient, err := metadata.NewForConfig(clientFactory.KubeConfig().RestConfig)
		if err != nil {
			return nil, fmt.Errorf("construct release metadata client: %w", err)
		}

		driver := helmdriver.NewConfigMaps(helmaction.NewConfigMapClient(lazyClient))
		driver.Log = logFn
		driver.MetadataClient = metadataClient
		driver.Namespace = namespace

		storage = helmstorage.Init(driver)
	case "memory":
		driver := helmdriver.NewMemory()
		driver.SetNamespace(namespace)

		storage = helmstorage.Init(driver)
	case "sql":
		driver, err := helmdriver.NewSQL(opts.SQLConnection, logFn, namespace)
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
