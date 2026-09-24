//go:build ai_tests

package release

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	helmdriver "github.com/werf/nelm/pkg/helm/pkg/storage/driver"
)

func TestAI_ConfigMapsRevisions_EmptyNamespaceIsRejected(t *testing.T) {
	cfgmaps := helmdriver.NewConfigMaps(nil)
	cfgmaps.MetadataClient = newMetadataClientForGVR(t, configMapsGVR, "ConfigMap")

	_, err := cfgmaps.Revisions(context.Background(), "myrelease")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
}

func TestAI_ConfigMapsRevisions_ReadsLabelsWithoutBodies(t *testing.T) {
	cfgmaps := helmdriver.NewConfigMaps(nil)
	cfgmaps.Namespace = testNamespace
	cfgmaps.MetadataClient = newMetadataClientForGVR(t, configMapsGVR, "ConfigMap",
		revisionLabels("myrelease", 2, helmreleasecommon.StatusDeployed.String()),
		revisionLabels("myrelease", 1, helmreleasecommon.StatusSuperseded.String()),
		map[string]string{"name": "myrelease", "owner": "helm", "version": "not-a-number"},
	)

	records, err := cfgmaps.Revisions(context.Background(), "myrelease")
	require.NoError(t, err)
	require.Len(t, records, 2)

	assert.Equal(t, []int{1, 2}, revisionVersions(records), "records must be sorted by ascending version")
	assert.Equal(t, helmreleasecommon.StatusSuperseded.String(), records[0].Status)
	assert.Equal(t, testNamespace, records[0].Namespace)
	assert.Equal(t, "myrelease", records[0].Name)
}

func TestAI_ConfigMapsRevisions_TypedListFallbackWhenNoMetadataClient(t *testing.T) {
	const relName = "myrelease"

	impl := k8sfake.NewSimpleClientset().CoreV1().ConfigMaps(testNamespace)
	cfgmaps := helmdriver.NewConfigMaps(impl)
	cfgmaps.Namespace = testNamespace

	require.NoError(t, cfgmaps.Create("sh.helm.release.v1."+relName+".v10", newTestReleaseWithStatus(relName, 10, helmreleasecommon.StatusDeployed)))
	require.NoError(t, cfgmaps.Create("sh.helm.release.v1."+relName+".v2", newTestReleaseWithStatus(relName, 2, helmreleasecommon.StatusSuperseded)))

	records, err := cfgmaps.Revisions(context.Background(), relName)
	require.NoError(t, err)
	require.Len(t, records, 2, "a nil metadata client must fall back to a typed list, not fail")

	assert.Equal(t, []int{2, 10}, revisionVersions(records), "fallback records must be sorted by ascending numeric version, not by the list's lexical key order")
	assert.Equal(t, helmreleasecommon.StatusSuperseded.String(), records[0].Status)
	assert.Equal(t, testNamespace, records[0].Namespace)
}

func TestAI_MemoryRevisions_EmptyNamespaceIsRejected(t *testing.T) {
	mem := helmdriver.NewMemory()
	mem.SetNamespace("")

	_, err := mem.Revisions(context.Background(), "myrelease")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
}

func TestAI_MemoryRevisions_ReturnsSortedRevisions(t *testing.T) {
	const relName = "myrelease"

	storage := newMemoryStorage(t,
		newTestReleaseWithStatus(relName, 3, helmreleasecommon.StatusDeployed),
		newTestReleaseWithStatus(relName, 1, helmreleasecommon.StatusSuperseded),
		newTestReleaseWithStatus(relName, 2, helmreleasecommon.StatusFailed),
	)

	mem, ok := storage.Driver.(*helmdriver.Memory)
	require.True(t, ok)

	records, err := mem.Revisions(context.Background(), relName)
	require.NoError(t, err)
	require.Len(t, records, 3)

	assert.Equal(t, []int{1, 2, 3}, revisionVersions(records), "records must be sorted by ascending version")
	assert.Equal(t, helmreleasecommon.StatusSuperseded.String(), records[0].Status)
	assert.Equal(t, helmreleasecommon.StatusFailed.String(), records[1].Status)
	assert.Equal(t, helmreleasecommon.StatusDeployed.String(), records[2].Status)
	assert.Equal(t, testNamespace, records[0].Namespace)
	assert.Equal(t, relName, records[0].Name)
}

func TestAI_MemoryRevisions_UnknownReleaseIsEmpty(t *testing.T) {
	mem := helmdriver.NewMemory()
	mem.SetNamespace(testNamespace)

	records, err := mem.Revisions(context.Background(), "absent")
	require.NoError(t, err)
	assert.Empty(t, records, "fresh driver holds no releases")
}

func TestAI_SecretsRevisions_EmptyNamespaceIsRejected(t *testing.T) {
	secrets := helmdriver.NewSecrets(nil)
	secrets.MetadataClient = newMetadataClient(t)

	_, err := secrets.Revisions(context.Background(), "myrelease")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
}

func TestAI_SecretsRevisions_ReadsLabelsWithoutBodies(t *testing.T) {
	secrets := helmdriver.NewSecrets(nil)
	secrets.Namespace = testNamespace
	secrets.MetadataClient = newMetadataClient(t,
		revisionLabels("myrelease", 2, helmreleasecommon.StatusDeployed.String()),
		revisionLabels("myrelease", 1, helmreleasecommon.StatusSuperseded.String()),
	)

	records, err := secrets.Revisions(context.Background(), "myrelease")
	require.NoError(t, err)
	require.Len(t, records, 2)

	assert.Equal(t, 1, records[0].Version, "records must be sorted by ascending version")
	assert.Equal(t, helmreleasecommon.StatusSuperseded.String(), records[0].Status)
	assert.Equal(t, 2, records[1].Version)
	assert.Equal(t, helmreleasecommon.StatusDeployed.String(), records[1].Status)
	assert.Equal(t, testNamespace, records[1].Namespace)
	assert.Equal(t, "myrelease", records[1].Name)
}

func TestAI_SecretsRevisions_SkipsUnparseableLabels(t *testing.T) {
	secrets := helmdriver.NewSecrets(nil)
	secrets.Namespace = testNamespace
	secrets.MetadataClient = newMetadataClient(t,
		revisionLabels("myrelease", 1, helmreleasecommon.StatusDeployed.String()),
		map[string]string{"name": "myrelease", "owner": "helm", "version": "not-a-number"},
		map[string]string{"owner": "helm", "version": "2"},
	)

	records, err := secrets.Revisions(context.Background(), "myrelease")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, 1, records[0].Version)
}

func TestAI_SecretsRevisions_TypedListFallbackWhenNoMetadataClient(t *testing.T) {
	const relName = "myrelease"

	storage, driver := newSecretStorage(t)
	driver.Namespace = testNamespace

	require.NoError(t, storage.Create(newTestReleaseWithStatus(relName, 10, helmreleasecommon.StatusDeployed)))
	require.NoError(t, storage.Create(newTestReleaseWithStatus(relName, 2, helmreleasecommon.StatusSuperseded)))
	require.NoError(t, storage.Create(newTestReleaseWithStatus(relName, 9, helmreleasecommon.StatusSuperseded)))
	require.NoError(t, storage.Create(newTestReleaseWithStatus("otherrelease", 5, helmreleasecommon.StatusDeployed)))

	records, err := driver.Revisions(context.Background(), relName)
	require.NoError(t, err)
	require.Len(t, records, 3, "a nil metadata client must fall back to a typed list, not fail")

	assert.Equal(t, []int{2, 9, 10}, revisionVersions(records), "fallback records must be sorted by ascending numeric version, not by the list's lexical key order")
	assert.Equal(t, helmreleasecommon.StatusSuperseded.String(), records[0].Status)
	assert.Equal(t, helmreleasecommon.StatusDeployed.String(), records[2].Status)
	assert.Equal(t, testNamespace, records[0].Namespace)
	assert.Equal(t, relName, records[0].Name)
}

func revisionLabels(name string, version int, status string) map[string]string {
	return map[string]string{
		"name":    name,
		"owner":   "helm",
		"version": strconv.Itoa(version),
		"status":  status,
	}
}

func revisionVersions(records []helmdriver.RevisionRecord) []int {
	versions := make([]int, 0, len(records))
	for _, record := range records {
		versions = append(versions, record.Version)
	}

	return versions
}
