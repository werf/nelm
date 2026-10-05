package release

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
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

// encodeHelmRelease encodes a body exactly as the Helm storage drivers do.
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

	return newSQLStorageBackendFromDB(sqlx.NewDb(db, sqlDialect)), mock
}

func newTestStoredObject(t *testing.T, rls *helmrelease.Release) *storedObject {
	t.Helper()

	obj, err := newStoredObject(rls.Namespace, rls, storageLabelCreatedAt)
	require.NoError(t, err)

	return obj
}

// putTestKubeObject stores obj in both fake clients: the typed one serves gets and body
// listings, the metadata one serves metadata listings, and the fakes share no state.
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
