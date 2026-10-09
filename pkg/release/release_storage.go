package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/werf/nelm/v2/pkg/common"
	v2release "github.com/werf/nelm/v2/pkg/helm/intern/release/v2"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/v2/pkg/kube"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/util"
)

const (
	ReleaseVersionV1 = "v1"
	ReleaseVersionV2 = "v2"
)

var (
	_ ReleaseStorager = (*releaseStorage)(nil)

	ErrReleaseExists      = errors.New("release: already exists")
	ErrReleaseNotFound    = errors.New("release: not found")
	ErrReleaseUndecodable = errors.New("release: stored body cannot be decoded")
)

// ReleaseStorager stores releases in the Helm format. Methods taking a release name require a
// namespace. Objects with an unparseable version label are skipped on reads but fail Revisions.
type ReleaseStorager interface {
	Create(ctx context.Context, rel helmrel.Accessor) error
	Update(ctx context.Context, rel helmrel.Accessor) error
	UpdateLabels(ctx context.Context, name string, version int, labels map[string]string) error
	Delete(ctx context.Context, name string, version int) error
	// GetRelease returns the decoded revision; version 0 means the latest one.
	GetRelease(ctx context.Context, name string, version int) (helmrel.Accessor, error)
	LoadRevision(ctx context.Context, revision Revision) (helmrel.Accessor, error)
	// LoadRevisionSummary fails with ErrReleaseUndecodable on an undecodable body.
	LoadRevisionSummary(ctx context.Context, revision Revision) (*ReleaseSummary, error)
	// Revisions reads no bodies; sorted by version.
	Revisions(ctx context.Context, name string) ([]Revision, error)
	// LatestRevisions reads no bodies; sorted by namespace and name.
	LatestRevisions(ctx context.Context, opts LatestRevisionsOptions) ([]Revision, error)
	// ListLatestSummaries streams bodies and keeps only summaries; sorted by namespace and name.
	ListLatestSummaries(ctx context.Context, opts ListLatestSummariesOptions) ([]RevisionSummary, error)
	// ListRevisionSummaries streams bodies and keeps only summaries; sorted by version.
	ListRevisionSummaries(ctx context.Context, name string, opts ListRevisionSummariesOptions) ([]RevisionSummary, error)
}

type storageBackend interface {
	get(ctx context.Context, namespace, key string) (*storedObject, error)
	create(ctx context.Context, obj *storedObject) error
	update(ctx context.Context, obj *storedObject) error
	updateLabels(ctx context.Context, namespace, key string, labels map[string]string) error
	delete(ctx context.Context, namespace, key string) error
	listMetadata(ctx context.Context, namespace, releaseName string) ([]*storedObject, error)
	// scanLatestCandidates may also pass older revisions, in any order.
	scanLatestCandidates(ctx context.Context, namespace string, selector labels.Selector, withBodies bool, fn func(obj *storedObject) error) error
	listWithBodies(ctx context.Context, namespace, releaseName string, versions []int, fn func(obj *storedObject) error) error
}

type ListLatestSummariesOptions struct {
	// LabelSelector is applied before choosing the latest revision.
	LabelSelector string
}

type LatestRevisionsOptions struct {
	// LabelSelector is applied before choosing the latest revision.
	LabelSelector string
}

type ListRevisionSummariesOptions struct {
	// Limit reads only the newest revisions; 0 means all.
	Limit int
}

type ReleaseStorageOptions struct {
	HistoryLimit  int
	SQLConnection string
}

type releaseStorage struct {
	backend      storageBackend
	historyLimit int
	namespace    string
}

func newReleaseStorage(namespace string, backend storageBackend, historyLimit int) *releaseStorage {
	return &releaseStorage{
		backend:      backend,
		historyLimit: historyLimit,
		namespace:    namespace,
	}
}

func (s *releaseStorage) Create(ctx context.Context, rel helmrel.Accessor) error {
	namespace, err := s.requireNamespace()
	if err != nil {
		return err
	}

	rls, err := ReleaserToV1Release(rel.Releaser())
	if err != nil {
		return fmt.Errorf("prepare release for storage: %w", err)
	}

	if s.historyLimit > 0 {
		if err := s.removeOldestRevisions(ctx, rls.Name, s.historyLimit-1); err != nil {
			return fmt.Errorf("remove oldest revisions of release %q: %w", rls.Name, err)
		}
	}

	obj, err := newStoredObject(namespace, rls, storageLabelCreatedAt)
	if err != nil {
		return err
	}

	if err := s.backend.create(ctx, obj); err != nil {
		return fmt.Errorf("create release object %q (namespace: %q): %w", obj.Key, namespace, err)
	}

	return nil
}

func (s *releaseStorage) Delete(ctx context.Context, name string, version int) error {
	namespace, err := s.requireNamespace()
	if err != nil {
		return err
	}

	key := storageKey(name, version)
	if err := s.backend.delete(ctx, namespace, key); err != nil {
		return fmt.Errorf("delete release object %q (namespace: %q): %w", key, namespace, err)
	}

	return nil
}

func (s *releaseStorage) GetRelease(ctx context.Context, name string, version int) (helmrel.Accessor, error) {
	namespace, err := s.requireNamespace()
	if err != nil {
		return nil, err
	}

	if name == "" {
		return nil, errors.New("release name is required")
	}

	if version != 0 {
		return s.LoadRevision(ctx, Revision{Name: name, Namespace: namespace, Version: version})
	}

	// A concurrent install may prune the latest revision before it is read.
	for attempt := 0; ; attempt++ {
		revisions, err := s.readableRevisions(ctx, namespace, name)
		if err != nil {
			return nil, err
		}

		if len(revisions) == 0 {
			return nil, ErrReleaseNotFound
		}

		rel, err := s.LoadRevision(ctx, revisions[len(revisions)-1])
		if err == nil || !errors.Is(err, ErrReleaseNotFound) || attempt > 0 {
			return rel, err
		}
	}
}

func (s *releaseStorage) LatestRevisions(ctx context.Context, opts LatestRevisionsOptions) ([]Revision, error) {
	selector, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("parse release label selector: %w", err)
	}

	latest := map[string]Revision{}
	if err := s.backend.scanLatestCandidates(ctx, s.namespace, selector, false, func(obj *storedObject) error {
		revision, ok, err := revisionFromStoredObject(obj)
		if err != nil {
			log.Default.Error(ctx, "Skipped storage object: %s", err)

			return nil
		}

		if !ok {
			return nil
		}

		id := revision.Namespace + "/" + revision.Name
		if current, found := latest[id]; found && current.Version >= revision.Version {
			return nil
		}

		latest[id] = revision

		return nil
	}); err != nil {
		return nil, fmt.Errorf("list release objects metadata: %w", err)
	}

	result := lo.Values(latest)

	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}

		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (s *releaseStorage) ListLatestSummaries(ctx context.Context, opts ListLatestSummariesOptions) ([]RevisionSummary, error) {
	selector, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("parse release label selector: %w", err)
	}

	latest := map[string]RevisionSummary{}

	var (
		candidate         *storedObject
		candidateRevision Revision
	)

	// Only one raw body, the current candidate, is retained at a time.
	summarize := func() {
		if candidate == nil {
			return
		}

		summary, decodeErr := decodeStoredSummary(candidate)
		latest[candidateRevision.Namespace+"/"+candidateRevision.Name] = RevisionSummary{
			DecodeErr: decodeErr,
			Revision:  candidateRevision,
			Summary:   summary,
		}
		candidate = nil
	}

	if err := s.backend.scanLatestCandidates(ctx, s.namespace, selector, true, func(obj *storedObject) error {
		revision, ok, err := revisionFromStoredObject(obj)
		if err != nil {
			log.Default.Error(ctx, "Skipped storage object: %s", err)

			return nil
		}

		if !ok {
			return nil
		}

		id := revision.Namespace + "/" + revision.Name
		if candidate != nil && id != candidateRevision.Namespace+"/"+candidateRevision.Name {
			summarize()
		}

		// Revisions of a release may be non-adjacent; an older one never replaces a newer one.
		if current, found := latest[id]; found && revision.Version <= current.Revision.Version {
			return nil
		}

		if candidate == nil || revision.Version > candidateRevision.Version {
			candidate, candidateRevision = obj, revision
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("list latest release objects: %w", err)
	}

	summarize()

	result := lo.Values(latest)

	sort.Slice(result, func(i, j int) bool {
		if result[i].Revision.Namespace != result[j].Revision.Namespace {
			return result[i].Revision.Namespace < result[j].Revision.Namespace
		}

		return result[i].Revision.Name < result[j].Revision.Name
	})

	return result, nil
}

func (s *releaseStorage) ListRevisionSummaries(ctx context.Context, name string, opts ListRevisionSummariesOptions) ([]RevisionSummary, error) {
	namespace, err := s.requireNamespace()
	if err != nil {
		return nil, err
	}

	if name == "" {
		return nil, errors.New("release name is required")
	}

	var versions []int
	if opts.Limit > 0 {
		revisions, err := s.readableRevisions(ctx, namespace, name)
		if err != nil {
			return nil, err
		}

		if len(revisions) == 0 {
			return nil, nil
		}

		if len(revisions) > opts.Limit {
			revisions = revisions[len(revisions)-opts.Limit:]
		}

		versions = lo.Map(revisions, func(revision Revision, _ int) int { return revision.Version })
	}

	var result []RevisionSummary
	if err := s.backend.listWithBodies(ctx, namespace, name, versions, func(obj *storedObject) error {
		revision, ok, err := revisionFromStoredObject(obj)
		if err != nil {
			log.Default.Error(ctx, "Skipped storage object of release %q: %s", name, err)

			return nil
		}

		if !ok {
			return nil
		}

		summary, decodeErr := decodeStoredSummary(obj)
		result = append(result, RevisionSummary{
			DecodeErr: decodeErr,
			Revision:  revision,
			Summary:   summary,
		})

		return nil
	}); err != nil {
		return nil, fmt.Errorf("list release objects of release %q (namespace: %q): %w", name, namespace, err)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Revision.Version < result[j].Revision.Version
	})

	return result, nil
}

func (s *releaseStorage) LoadRevision(ctx context.Context, revision Revision) (helmrel.Accessor, error) {
	if revision.Namespace == "" {
		return nil, fmt.Errorf("load revision %d of release %q: namespace is required", revision.Version, revision.Name)
	}

	key := storageKey(revision.Name, revision.Version)

	obj, err := s.backend.get(ctx, revision.Namespace, key)
	if err != nil {
		return nil, fmt.Errorf("get release object %q (namespace: %q): %w", key, revision.Namespace, err)
	}

	rls, err := decodeStoredObject(obj)
	if err != nil {
		return nil, err
	}

	acc, err := helmrel.NewAccessor(rls)
	if err != nil {
		return nil, fmt.Errorf("wrap release: %w", err)
	}

	return acc, nil
}

func (s *releaseStorage) LoadRevisionSummary(ctx context.Context, revision Revision) (*ReleaseSummary, error) {
	if revision.Namespace == "" {
		return nil, fmt.Errorf("load revision %d of release %q: namespace is required", revision.Version, revision.Name)
	}

	key := storageKey(revision.Name, revision.Version)

	obj, err := s.backend.get(ctx, revision.Namespace, key)
	if err != nil {
		return nil, fmt.Errorf("get release object %q (namespace: %q): %w", key, revision.Namespace, err)
	}

	return decodeStoredSummary(obj)
}

func (s *releaseStorage) Revisions(ctx context.Context, name string) ([]Revision, error) {
	namespace, err := s.requireNamespace()
	if err != nil {
		return nil, err
	}

	if name == "" {
		return nil, errors.New("release name is required")
	}

	objects, err := s.backend.listMetadata(ctx, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("list storage metadata of release %q (namespace: %q): %w", name, namespace, err)
	}

	revisions := make([]Revision, 0, len(objects))
	for _, obj := range objects {
		revision, ok, err := revisionFromStoredObject(obj)
		if err != nil {
			return nil, fmt.Errorf("list revisions of release %q: %w", name, err)
		}

		if !ok {
			continue
		}

		revisions = append(revisions, revision)
	}

	sort.Slice(revisions, func(i, j int) bool { return revisions[i].Version < revisions[j].Version })

	return revisions, nil
}

func (s *releaseStorage) Update(ctx context.Context, rel helmrel.Accessor) error {
	namespace, err := s.requireNamespace()
	if err != nil {
		return err
	}

	rls, err := ReleaserToV1Release(rel.Releaser())
	if err != nil {
		return fmt.Errorf("prepare release for storage: %w", err)
	}

	obj, err := newStoredObject(namespace, rls, storageLabelModifiedAt)
	if err != nil {
		return err
	}

	if err := s.backend.update(ctx, obj); err != nil {
		return fmt.Errorf("update release object %q (namespace: %q): %w", obj.Key, namespace, err)
	}

	return nil
}

func (s *releaseStorage) UpdateLabels(ctx context.Context, name string, version int, labels map[string]string) error {
	namespace, err := s.requireNamespace()
	if err != nil {
		return err
	}

	key := storageKey(name, version)
	if err := s.backend.updateLabels(ctx, namespace, key, withoutSystemLabels(labels)); err != nil {
		return fmt.Errorf("update labels of release object %q (namespace: %q): %w", key, namespace, err)
	}

	return nil
}

func (s *releaseStorage) readableRevisions(ctx context.Context, namespace, name string) ([]Revision, error) {
	objects, err := s.backend.listMetadata(ctx, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("list storage metadata of release %q (namespace: %q): %w", name, namespace, err)
	}

	revisions := make([]Revision, 0, len(objects))
	for _, obj := range objects {
		revision, ok, err := revisionFromStoredObject(obj)
		if err != nil {
			log.Default.Error(ctx, "Skipped storage object of release %q: %s", name, err)

			continue
		}

		if !ok {
			continue
		}

		revisions = append(revisions, revision)
	}

	sort.Slice(revisions, func(i, j int) bool { return revisions[i].Version < revisions[j].Version })

	return revisions, nil
}

func (s *releaseStorage) removeOldestRevisions(ctx context.Context, name string, maximum int) error {
	if maximum < 0 {
		return nil
	}

	revisions, err := s.Revisions(ctx, name)
	if err != nil {
		return err
	}

	if len(revisions) <= maximum {
		return nil
	}

	lastDeployed, lastDeployedFound := 0, false
	for _, revision := range revisions {
		if revision.Status == helmreleasecommon.StatusDeployed.String() {
			lastDeployed, lastDeployedFound = revision.Version, true
		}
	}

	var toDelete []Revision
	for _, revision := range revisions {
		if len(revisions)-len(toDelete) == maximum {
			break
		}

		if lastDeployedFound && revision.Version == lastDeployed {
			continue
		}

		toDelete = append(toDelete, revision)
	}

	errs := &util.MultiError{}
	for _, revision := range toDelete {
		if err := s.Delete(ctx, name, revision.Version); err != nil && !errors.Is(err, ErrReleaseNotFound) {
			errs.Add(err)
		}
	}

	return errs.OrNilIfNoErrs()
}

func (s *releaseStorage) requireNamespace() (string, error) {
	if s.namespace == "" {
		return "", errors.New("release storage namespace is required")
	}

	return s.namespace, nil
}

func NewReleaseStorage(ctx context.Context, namespace, storageDriver string, clientFactory kube.ClientFactorier, opts ReleaseStorageOptions) (ReleaseStorager, error) {
	var backend storageBackend

	switch storageDriver {
	case common.ReleaseStorageDriverSecret, common.ReleaseStorageDriverSecrets, common.ReleaseStorageDriverDefault,
		common.ReleaseStorageDriverConfigMap, common.ReleaseStorageDriverConfigMaps:
		if clientFactory == nil {
			return nil, fmt.Errorf("kube client factory is required for %q storage driver", storageDriver)
		}

		kind := kubeStorageKindSecret
		if storageDriver == common.ReleaseStorageDriverConfigMap || storageDriver == common.ReleaseStorageDriverConfigMaps {
			kind = kubeStorageKindConfigMap
		}

		backend = newKubeStorageBackend(kind, clientFactory.Static(), clientFactory.Metadata())
	case common.ReleaseStorageDriverMemory:
		backend = newMemoryStorageBackend()
	case common.ReleaseStorageDriverSQL:
		sqlBackend, err := openSQLStorageBackend(ctx, opts.SQLConnection)
		if err != nil {
			return nil, fmt.Errorf("construct sql release storage: %w", err)
		}

		backend = sqlBackend
	default:
		panic(fmt.Sprintf("Unknown storage driver: %s", storageDriver))
	}

	return newReleaseStorage(namespace, backend, opts.HistoryLimit), nil
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
