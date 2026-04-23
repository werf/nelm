package release

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/werf/nelm/pkg/common"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	helmdriver "github.com/werf/nelm/pkg/helm/pkg/storage/driver"
	"github.com/werf/nelm/pkg/kube"
)

var _ ReleaseStorager = (*storageAdapter)(nil)

type ReleaseStorager interface {
	Create(rls *helmrelease.Release) error
	Update(rls *helmrelease.Release) error
	Delete(name string, version int) (*helmrelease.Release, error)
	Query(labels map[string]string) ([]*helmrelease.Release, error)
}

type storageAdapter struct {
	storage *helmstorage.Storage
}

func (a *storageAdapter) Create(rls *helmrelease.Release) error {
	if err := a.storage.Create(rls); err != nil {
		return fmt.Errorf("create release: %w", err)
	}

	return nil
}

func (a *storageAdapter) Delete(name string, version int) (*helmrelease.Release, error) {
	rel, err := a.storage.Delete(name, version)
	if err != nil {
		return nil, fmt.Errorf("delete release: %w", err)
	}

	r, ok := rel.(*helmrelease.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected release type: %T", rel)
	}

	return r, nil
}

func (a *storageAdapter) Query(labels map[string]string) ([]*helmrelease.Release, error) {
	releasers, err := a.storage.Query(labels)
	if err != nil {
		return nil, fmt.Errorf("query releases: %w", err)
	}

	result := make([]*helmrelease.Release, 0, len(releasers))
	for _, rel := range releasers {
		r, ok := rel.(*helmrelease.Release)
		if !ok {
			return nil, fmt.Errorf("unexpected release type: %T", rel)
		}

		result = append(result, r)
	}

	return result, nil
}

func (a *storageAdapter) Storage() *helmstorage.Storage {
	return a.storage
}

func (a *storageAdapter) Update(rls *helmrelease.Release) error {
	if err := a.storage.Update(rls); err != nil {
		return fmt.Errorf("update release: %w", err)
	}

	return nil
}

type ReleaseStorageOptions struct {
	HistoryLimit  int
	SQLConnection string
}

func NewReleaseStorage(ctx context.Context, namespace, storageDriver string, clientFactory kube.ClientFactorier, opts ReleaseStorageOptions) (ReleaseStorager, error) {
	var storage *helmstorage.Storage

	switch storageDriver {
	case common.ReleaseStorageDriverSecret, common.ReleaseStorageDriverSecrets, common.ReleaseStorageDriverDefault:
		if clientFactory == nil {
			return nil, fmt.Errorf("kube client factory is required for %q storage driver", storageDriver)
		}

		clientset := clientFactory.Static().(*kubernetes.Clientset)
		d := helmdriver.NewSecrets(clientset.CoreV1().Secrets(namespace))
		storage = helmstorage.Init(d)
	case common.ReleaseStorageDriverConfigMap, common.ReleaseStorageDriverConfigMaps:
		if clientFactory == nil {
			return nil, fmt.Errorf("kube client factory is required for %q storage driver", storageDriver)
		}

		clientset := clientFactory.Static().(*kubernetes.Clientset)
		d := helmdriver.NewConfigMaps(clientset.CoreV1().ConfigMaps(namespace))
		storage = helmstorage.Init(d)
	case common.ReleaseStorageDriverMemory:
		d := helmdriver.NewMemory()
		d.SetNamespace(namespace)
		storage = helmstorage.Init(d)
	case common.ReleaseStorageDriverSQL:
		d, err := helmdriver.NewSQL(opts.SQLConnection, namespace)
		if err != nil {
			return nil, fmt.Errorf("construct sql driver: %w", err)
		}

		storage = helmstorage.Init(d)
	default:
		panic(fmt.Sprintf("Unknown storage driver: %s", storageDriver))
	}

	storage.MaxHistory = opts.HistoryLimit

	return &storageAdapter{storage: storage}, nil
}
