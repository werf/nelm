package release

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/werf/nelm/pkg/common"
	v2release "github.com/werf/nelm/pkg/helm/intern/release/v2"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	helmdriver "github.com/werf/nelm/pkg/helm/pkg/storage/driver"
	"github.com/werf/nelm/pkg/kube"
)

const (
	ReleaseVersionV1 = "v1"
	ReleaseVersionV2 = "v2"
)

var _ ReleaseStorager = (*storageAdapter)(nil)

type ReleaseStorager interface {
	Create(rls helmrel.Accessor) error
	Update(rls helmrel.Accessor) error
	Delete(name string, version int) (helmrel.Accessor, error)
	Query(labels map[string]string) ([]helmrel.Accessor, error)
}

type storageAdapter struct {
	storage *helmstorage.Storage
}

func (a *storageAdapter) Create(rls helmrel.Accessor) error {
	releaser, err := ReleaserToV1Release(rls.Releaser())
	if err != nil {
		return fmt.Errorf("prepare release for storage: %w", err)
	}

	if err := a.storage.Create(releaser); err != nil {
		return fmt.Errorf("create release: %w", err)
	}

	return nil
}

func (a *storageAdapter) Delete(name string, version int) (helmrel.Accessor, error) {
	rel, err := a.storage.Delete(name, version)
	if err != nil {
		return nil, fmt.Errorf("delete release: %w", err)
	}

	acc, err := helmrel.NewAccessor(rel)
	if err != nil {
		return nil, fmt.Errorf("wrap release: %w", err)
	}

	return acc, nil
}

func (a *storageAdapter) Query(labels map[string]string) ([]helmrel.Accessor, error) {
	releasers, err := a.storage.Query(labels)
	if err != nil {
		return nil, fmt.Errorf("query releases: %w", err)
	}

	result := make([]helmrel.Accessor, 0, len(releasers))
	for _, rel := range releasers {
		acc, err := helmrel.NewAccessor(rel)
		if err != nil {
			return nil, fmt.Errorf("wrap release: %w", err)
		}

		result = append(result, acc)
	}

	return result, nil
}

func (a *storageAdapter) Storage() *helmstorage.Storage {
	return a.storage
}

func (a *storageAdapter) Update(rls helmrel.Accessor) error {
	releaser, err := ReleaserToV1Release(rls.Releaser())
	if err != nil {
		return fmt.Errorf("prepare release for storage: %w", err)
	}

	if err := a.storage.Update(releaser); err != nil {
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

func ReleaserToV1Release(releaser helmrel.Releaser) (*helmrelease.Release, error) {
	switch r := releaser.(type) {
	case *helmrelease.Release:
		return r, nil
	case *v2release.Release:
		v1rel, err := v2ReleaseToV1Release(r)
		if err != nil {
			return nil, fmt.Errorf("convert v2 release to v1 release: %w", err)
		}

		return v1rel, nil
	default:
		return nil, fmt.Errorf("unexpected release type: %T", releaser)
	}
}

func ReleaserVersion(releaser helmrel.Releaser) string {
	switch releaser.(type) {
	case *helmrelease.Release:
		return ReleaseVersionV1
	case *v2release.Release:
		return ReleaseVersionV2
	default:
		panic(fmt.Sprintf("unexpected release type: %T", releaser))
	}
}

func V1ReleaseToV2Release(rel *helmrelease.Release) (*v2release.Release, error) {
	data, err := json.Marshal(rel)
	if err != nil {
		return nil, fmt.Errorf("marshal v1 release: %w", err)
	}

	v2rel := &v2release.Release{}
	if err := json.Unmarshal(data, v2rel); err != nil {
		return nil, fmt.Errorf("unmarshal into v2 release: %w", err)
	}

	v2rel.Labels = rel.Labels

	return v2rel, nil
}

func v2ReleaseToV1Release(rel *v2release.Release) (*helmrelease.Release, error) {
	data, err := json.Marshal(rel)
	if err != nil {
		return nil, fmt.Errorf("marshal v2 release: %w", err)
	}

	v1rel := &helmrelease.Release{}
	if err := json.Unmarshal(data, v1rel); err != nil {
		return nil, fmt.Errorf("unmarshal into v1 release: %w", err)
	}

	v1rel.Labels = rel.Labels

	return v1rel, nil
}
