package release

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
)

const testNamespace = "test-ns"

type testKubeStorage struct {
	backend        *kubeStorageBackend
	client         *k8sfake.Clientset
	metadataClient *metadatafake.FakeMetadataClient
	storage        *releaseStorage
}

func newTestKubeStorage(t *testing.T, kind kubeStorageKind, namespace string, historyLimit int) *testKubeStorage {
	t.Helper()

	scheme := metadatafake.NewTestScheme()
	require.NoError(t, metav1.AddMetaToScheme(scheme))

	client := k8sfake.NewClientset()
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)
	backend := newKubeStorageBackend(kind, client, metadataClient)

	return &testKubeStorage{
		backend:        backend,
		client:         client,
		metadataClient: metadataClient,
		storage:        newReleaseStorage(namespace, backend, historyLimit),
	}
}

type latestListingBackend struct {
	storageBackend

	err        error
	generate   func(fn func(*storedObject) error) error
	listings   int
	objects    []*storedObject
	selector   labels.Selector
	withBodies bool
}

func (b *latestListingBackend) scanLatestCandidates(ctx context.Context, namespace string, selector labels.Selector, withBodies bool, fn func(*storedObject) error) error {
	b.listings++
	b.withBodies = withBodies

	b.selector = selector
	if b.err != nil {
		return b.err
	}

	if b.generate != nil {
		return b.generate(fn)
	}

	for _, obj := range b.objects {
		if namespace != "" && obj.Namespace != namespace {
			continue
		}

		if obj.Labels[storageLabelOwner] != storageOwner || !selector.Matches(labels.Set(obj.Labels)) {
			continue
		}

		if err := fn(obj); err != nil {
			return err
		}
	}

	return nil
}

// The client-go metadata fake drops continue tokens.
type pagedMetadataClient struct {
	metadata.ResourceInterface

	pages    map[string]*metav1.PartialObjectMetadataList
	requests []metav1.ListOptions
}

func (c *pagedMetadataClient) List(ctx context.Context, opts metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
	if lo.ContainsBy(c.requests, func(previous metav1.ListOptions) bool { return previous.Continue == opts.Continue }) {
		return nil, fmt.Errorf("page with continue token %q requested twice", opts.Continue)
	}

	c.requests = append(c.requests, opts)

	page, found := c.pages[opts.Continue]
	if !found {
		return nil, fmt.Errorf("unexpected continue token %q", opts.Continue)
	}

	return page, nil
}

func (c *pagedMetadataClient) Namespace(string) metadata.ResourceInterface {
	return c
}

func (c *pagedMetadataClient) Resource(schema.GroupVersionResource) metadata.Getter {
	return c
}

func newTestReleaseAccessor(t *testing.T, name string, version int, status helmreleasecommon.Status) helmrel.Accessor {
	t.Helper()

	acc, err := helmrel.NewAccessor(newTestReleaseWithStatus(name, version, status))
	require.NoError(t, err)

	return acc
}

func newMemoryReleaseStorage(t *testing.T, rels ...*helmrelease.Release) *releaseStorage {
	t.Helper()

	backend := newMemoryStorageBackend()
	for _, rls := range rels {
		require.NoError(t, backend.create(context.Background(), newTestStoredObject(t, rls)))
	}

	return newReleaseStorage(testNamespace, backend, 0)
}

func newTestReleaseWithStatus(name string, version int, status helmreleasecommon.Status) *helmrelease.Release {
	return newTestRelease(testNamespace, name, version, status)
}

func newUndecodableTestStoredObject(t *testing.T, namespace, name string, version int, status helmreleasecommon.Status) *storedObject {
	t.Helper()

	obj := newTestStoredObject(t, newTestRelease(namespace, name, version, status))
	obj.Body = []byte("not a release body")

	return obj
}

func putTestKubeRelease(t *testing.T, s *testKubeStorage, rls *helmrelease.Release) {
	t.Helper()

	putTestKubeObject(t, s, newTestStoredObject(t, rls))
}

func countActions(actions []k8stesting.Action, verb string) int {
	var count int
	for _, action := range actions {
		if action.GetVerb() == verb {
			count++
		}
	}

	return count
}

func encodeHelmRelease(t *testing.T, rls *helmrelease.Release) []byte {
	t.Helper()

	data, err := json.Marshal(rls)
	require.NoError(t, err)

	var buf bytes.Buffer

	writer, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	require.NoError(t, err)

	_, err = writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return []byte(base64.StdEncoding.EncodeToString(buf.Bytes()))
}

func listTestKubeSecrets(t *testing.T, s *testKubeStorage, namespace string) []corev1.Secret {
	t.Helper()

	list, err := s.client.Tracker().List(corev1.SchemeGroupVersion.WithResource("secrets"), corev1.SchemeGroupVersion.WithKind("Secret"), namespace)
	require.NoError(t, err)

	secretList, ok := list.(*corev1.SecretList)
	require.True(t, ok)

	return secretList.Items
}

func newTestMetadataPage(next string, objectLabels ...map[string]string) *metav1.PartialObjectMetadataList {
	list := &metav1.PartialObjectMetadataList{ListMeta: metav1.ListMeta{Continue: next}}
	for _, labels := range objectLabels {
		list.Items = append(list.Items, metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "sh.helm.release.v1." + labels["name"] + ".v" + labels["version"], Labels: labels},
		})
	}

	return list
}

func newTestRelease(namespace, name string, version int, status helmreleasecommon.Status) *helmrelease.Release {
	return &helmrelease.Release{
		Name:      name,
		Namespace: namespace,
		Version:   version,
		Info:      &helmrelease.Info{Status: status},
	}
}

func newTestSQLBackend(t *testing.T) (*sqlStorageBackend, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	})

	return newSQLStorageBackend(sqlx.NewDb(db, sqlDialect)), mock
}

func newTestStoredObject(t *testing.T, rls *helmrelease.Release) *storedObject {
	t.Helper()

	obj, err := newStoredObject(rls.Namespace, rls, storageLabelCreatedAt)
	require.NoError(t, err)

	return obj
}

// The typed and metadata fakes share no state.
func putTestKubeObject(t *testing.T, s *testKubeStorage, obj *storedObject) {
	t.Helper()

	require.NoError(t, s.backend.create(context.Background(), obj))

	kind := "Secret"
	if s.backend.kind == kubeStorageKindConfigMap {
		kind = "ConfigMap"
	}

	metadataObj := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kind},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.Namespace,
			Name:      obj.Key,
			Labels:    obj.Labels,
		},
	}

	_, err := s.metadataClient.Resource(s.backend.gvr()).Namespace(obj.Namespace).(metadatafake.MetadataClient).CreateFake(metadataObj, metav1.CreateOptions{})
	require.NoError(t, err)

	s.client.ClearActions()
	s.metadataClient.ClearActions()
}

func summaryVersions(t *testing.T, summaries []RevisionSummary) []int {
	t.Helper()

	versions := make([]int, 0, len(summaries))
	for _, summary := range summaries {
		require.NoError(t, summary.DecodeErr)

		versions = append(versions, summary.Revision.Version)
	}

	return versions
}

func testLatestBodySelectors(t *testing.T, create func(*storedObject), list func(context.Context, string, labels.Selector, func(*storedObject) error) error) {
	t.Helper()

	for _, obj := range []*storedObject{
		{Namespace: "ns-a", Key: "a-v1", Labels: map[string]string{"owner": "helm", "name": "a", "version": "1", "packageChecksum": "old"}, Body: []byte("a-v1")},
		{Namespace: "ns-a", Key: "a-v2", Labels: map[string]string{"owner": "helm", "name": "a", "version": "2", "packageChecksum": "new"}, Body: []byte("a-v2")},
		{Namespace: "ns-b", Key: "b-v1", Labels: map[string]string{"owner": "helm", "name": "b", "version": "1"}, Body: []byte("b-v1")},
		{Namespace: "ns-a", Key: "foreign", Labels: map[string]string{"owner": "other", "packageChecksum": "old"}, Body: []byte("foreign")},
		{Namespace: "ns-a", Key: "unowned", Labels: map[string]string{"packageChecksum": "old"}, Body: []byte("unowned")},
	} {
		create(obj)
	}

	for _, tt := range []struct {
		name      string
		namespace string
		selector  string
		keys      []string
	}{
		{name: "all candidates", keys: []string{"ns-a/a-v1", "ns-a/a-v2", "ns-b/b-v1"}},
		{name: "exists", selector: "packageChecksum", keys: []string{"ns-a/a-v1", "ns-a/a-v2"}},
		{name: "equality before maximum", selector: "packageChecksum=old", keys: []string{"ns-a/a-v1"}},
		{name: "set", selector: "packageChecksum in (old,new)", keys: []string{"ns-a/a-v1", "ns-a/a-v2"}},
		{name: "missing notin", selector: "packageChecksum notin (old,new)", keys: []string{"ns-b/b-v1"}},
		{name: "namespace", namespace: "ns-a", keys: []string{"ns-a/a-v1", "ns-a/a-v2"}},
		{name: "owner contradiction", selector: "owner=other"},
		{name: "owner exclusion", selector: "owner!=helm"},
		{name: "owner set contradiction", selector: "owner in (other)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selector, err := labels.Parse(tt.selector)
			require.NoError(t, err)

			renderedSelector := selector.String()

			var keys []string
			require.NoError(t, list(context.Background(), tt.namespace, selector, func(obj *storedObject) error {
				keys = append(keys, obj.Namespace+"/"+obj.Key)
				assert.Equal(t, obj.Key, string(obj.Body))

				return nil
			}))
			assert.ElementsMatch(t, tt.keys, keys)
			assert.Equal(t, renderedSelector, selector.String())
		})
	}

	t.Run("nothing", func(t *testing.T) {
		require.NoError(t, list(context.Background(), "", labels.Nothing(), func(*storedObject) error {
			t.Fatal("Nothing must not match any object")

			return nil
		}))
	})
}
