//go:build ai_tests

package release

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	helmdriver "github.com/werf/nelm/pkg/helm/pkg/storage/driver"
)

const testNamespace = "test-ns"

var secretsGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

func TestAI_StorageGetRelease_LatestViaMetadata(t *testing.T) {
	const relName = "myrel"

	storage, driver := newSecretStorage(t,
		newTestRelease(relName, 1, nil),
		newTestRelease(relName, 2, nil),
		newTestRelease(relName, 3, map[string]string{"moduleChecksum": "ccc"}),
	)

	driver.MetadataClient = newMetadataClient(t, versionLabelSets(relName, 1, 2, 3)...)
	driver.Namespace = testNamespace

	rel, err := storage.GetRelease(relName, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, rel.Version)
	assert.Equal(t, "ccc", rel.Labels["moduleChecksum"])
	assert.Equal(t, "helm", rel.Labels["owner"], "fetch via Query keeps unfiltered system labels")
}

func TestAI_StorageGetRelease_MemoryDriver(t *testing.T) {
	const relName = "myrel"

	var rels []*helmrelease.Release
	for v := 1; v <= 11; v++ {
		rels = append(rels, newTestRelease(relName, v, nil))
	}

	storage := newMemoryStorage(t, rels...)

	latest, err := storage.GetRelease(relName, 0)
	require.NoError(t, err)
	assert.Equal(t, 11, latest.Version, "memory LastVersion must pick numeric max revision")

	specific, err := storage.GetRelease(relName, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, specific.Version)
}

func TestAI_StorageGetRelease_MetadataNumericMax(t *testing.T) {
	const relName = "myrel"

	var rels []*helmrelease.Release
	for v := 1; v <= 11; v++ {
		rels = append(rels, newTestRelease(relName, v, nil))
	}

	storage, driver := newSecretStorage(t, rels...)

	labelSets := versionLabelSets(relName, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11)
	labelSets = append(labelSets,
		map[string]string{"owner": "helm", "name": relName, "version": "not-a-number"},
		map[string]string{"owner": "helm", "name": "otherrel", "version": "99"},
	)

	driver.MetadataClient = newMetadataClient(t, labelSets...)
	driver.Namespace = testNamespace

	rel, err := storage.GetRelease(relName, 0)
	require.NoError(t, err)
	assert.Equal(t, 11, rel.Version, "must pick numeric max (11), not lexical max (9); non-integer and other-release metadata ignored")
}

func TestAI_StorageGetRelease_NotFound(t *testing.T) {
	storage, driver := newSecretStorage(t)
	driver.MetadataClient = newMetadataClient(t)
	driver.Namespace = testNamespace

	_, err := storage.GetRelease("absent", 0)
	require.ErrorIs(t, err, helmdriver.ErrReleaseNotFound)

	memoryStorage := newMemoryStorage(t)

	_, err = memoryStorage.GetRelease("absent", 0)
	require.ErrorIs(t, err, helmdriver.ErrReleaseNotFound)
}

func TestAI_StorageGetRelease_SpecificRevisionPreservesSystemLabels(t *testing.T) {
	const relName = "myrel"

	storage, _ := newSecretStorage(t,
		newTestRelease(relName, 1, map[string]string{"moduleChecksum": "aaa"}),
		newTestRelease(relName, 2, map[string]string{"moduleChecksum": "bbb"}),
		newTestRelease(relName, 3, map[string]string{"moduleChecksum": "ccc"}),
	)

	rel, err := storage.GetRelease(relName, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, rel.Version)
	assert.Equal(t, relName, rel.Labels["name"])
	assert.Equal(t, "helm", rel.Labels["owner"])
	assert.Equal(t, "deployed", rel.Labels["status"])
	assert.Equal(t, "2", rel.Labels["version"])
	assert.Equal(t, "bbb", rel.Labels["moduleChecksum"])

	stripped, err := storage.Get(relName, 2)
	require.NoError(t, err)
	assert.NotContains(t, stripped.Labels, "owner", "Storage.Get strips system labels; GetRelease must fetch via Query instead")
	assert.Equal(t, "bbb", stripped.Labels["moduleChecksum"])
}

func TestAI_StorageGetRelease_TypedListFallbackWhenNoMetadataClient(t *testing.T) {
	const relName = "myrel"

	var rels []*helmrelease.Release
	for v := 1; v <= 11; v++ {
		rels = append(rels, newTestRelease(relName, v, nil))
	}

	storage, _ := newSecretStorage(t, rels...)

	rel, err := storage.GetRelease(relName, 0)
	require.NoError(t, err)
	assert.Equal(t, 11, rel.Version, "without a metadata client LastVersion falls back to a typed list and still picks numeric max")
}

func newMemoryStorage(t *testing.T, rels ...*helmrelease.Release) *helmstorage.Storage {
	t.Helper()

	mem := helmdriver.NewMemory()
	mem.SetNamespace(testNamespace)
	storage := helmstorage.Init(mem)

	for _, rel := range rels {
		require.NoError(t, storage.Create(rel))
	}

	return storage
}

func newMetadataClient(t *testing.T, labelSets ...map[string]string) *metadatafake.FakeMetadataClient {
	t.Helper()

	scheme := metadatafake.NewTestScheme()
	require.NoError(t, metav1.AddMetaToScheme(scheme))

	client := metadatafake.NewSimpleMetadataClient(scheme)
	resourceClient := client.Resource(secretsGVR).Namespace(testNamespace).(metadatafake.MetadataClient)

	for i, labels := range labelSets {
		obj := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      fmt.Sprintf("obj-%d", i),
				Labels:    labels,
			},
		}

		_, err := resourceClient.CreateFake(obj, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	return client
}

func newSecretStorage(t *testing.T, rels ...*helmrelease.Release) (*helmstorage.Storage, *helmdriver.Secrets) {
	t.Helper()

	secrets := k8sfake.NewSimpleClientset().CoreV1().Secrets(testNamespace)
	driver := helmdriver.NewSecrets(secrets)
	storage := helmstorage.Init(driver)

	for _, rel := range rels {
		require.NoError(t, storage.Create(rel))
	}

	return storage, driver
}

func newTestRelease(name string, version int, labels map[string]string) *helmrelease.Release {
	return &helmrelease.Release{
		Name:      name,
		Namespace: testNamespace,
		Version:   version,
		Info:      &helmrelease.Info{Status: helmrelease.StatusDeployed},
		Labels:    labels,
	}
}

func versionLabelSets(name string, versions ...int) []map[string]string {
	var sets []map[string]string
	for _, v := range versions {
		sets = append(sets, map[string]string{"owner": "helm", "name": name, "version": strconv.Itoa(v)})
	}

	return sets
}
