//go:build ai_tests

package release //nolint:testpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	helmdriver "github.com/werf/nelm/pkg/helm/pkg/storage/driver"
)

var _ helmdriver.Driver = (*plainDriver)(nil)

// plainDriver forwards to the memory driver without exposing ListReleaseMeta, so
// that Storage has to take its fallback path. The methods are forwarded
// explicitly rather than embedded, because embedding would promote the
// capability and defeat the point.
type plainDriver struct {
	inner *helmdriver.Memory
}

func (d *plainDriver) Create(key string, rls *helmrelease.Release) error {
	return d.inner.Create(key, rls) //nolint:wrapcheck
}

func (d *plainDriver) Delete(key string) (*helmrelease.Release, error) {
	return d.inner.Delete(key) //nolint:wrapcheck
}

func (d *plainDriver) Get(key string) (*helmrelease.Release, error) {
	return d.inner.Get(key) //nolint:wrapcheck
}

func (d *plainDriver) List(filter func(*helmrelease.Release) bool) ([]*helmrelease.Release, error) {
	return d.inner.List(filter) //nolint:wrapcheck
}

func (d *plainDriver) Name() string {
	return d.inner.Name()
}

func (d *plainDriver) Query(labels map[string]string) ([]*helmrelease.Release, error) {
	return d.inner.Query(labels) //nolint:wrapcheck
}

func (d *plainDriver) Update(key string, rls *helmrelease.Release) error {
	return d.inner.Update(key, rls) //nolint:wrapcheck
}

func TestAI_StorageListReleaseMeta_Empty(t *testing.T) {
	storage, driver := newSecretStorage(t)
	driver.MetadataClient = newMetadataClient(t)
	driver.Namespace = testNamespace

	metas, err := storage.ListReleaseMeta()
	require.NoError(t, err)
	assert.Empty(t, metas, "an empty storage is not an error for a listing")

	memoryStorage := newMemoryStorage(t)

	metas, err = memoryStorage.ListReleaseMeta()
	require.NoError(t, err)
	assert.Empty(t, metas)
}

func TestAI_StorageListReleaseMeta_FallbackForDriverWithoutCapability(t *testing.T) {
	mem := helmdriver.NewMemory()
	mem.SetNamespace(testNamespace)

	storage := helmstorage.Init(&plainDriver{inner: mem})
	require.NoError(t, storage.Create(newTestRelease("one", 1, nil)))
	require.NoError(t, storage.Create(newTestRelease("one", 2, nil)))

	metas, err := storage.ListReleaseMeta()
	require.NoError(t, err)

	assert.ElementsMatch(t, []helmdriver.ReleaseMeta{
		{Name: "one", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 1},
		{Name: "one", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 2},
	}, metas)
}

func TestAI_StorageListReleaseMeta_FallbackForEmptyDriverWithoutCapability(t *testing.T) {
	mem := helmdriver.NewMemory()
	mem.SetNamespace(testNamespace)

	storage := helmstorage.Init(&plainDriver{inner: mem})

	metas, err := storage.ListReleaseMeta()
	require.NoError(t, err)
	assert.Empty(t, metas, "ErrReleaseNotFound from the driver means an empty listing, not a failure")
}

func TestAI_StorageListReleaseMeta_MemoryDriver(t *testing.T) {
	storage := newMemoryStorage(t,
		newTestRelease("one", 1, nil),
		newTestRelease("one", 2, nil),
		newTestRelease("two", 1, nil),
	)

	metas, err := storage.ListReleaseMeta()
	require.NoError(t, err)

	assert.ElementsMatch(t, []helmdriver.ReleaseMeta{
		{Name: "one", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 1},
		{Name: "one", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 2},
		{Name: "two", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 1},
	}, metas)
}

func TestAI_StorageListReleaseMeta_TypedListFallbackWhenNoMetadataClient(t *testing.T) {
	storage, _ := newSecretStorage(t,
		newTestRelease("one", 1, nil),
		newTestRelease("one", 2, nil),
		newTestRelease("two", 7, nil),
	)

	metas, err := storage.ListReleaseMeta()
	require.NoError(t, err)

	assert.ElementsMatch(t, []helmdriver.ReleaseMeta{
		{Name: "one", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 1},
		{Name: "one", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 2},
		{Name: "two", Namespace: testNamespace, Status: helmrelease.StatusDeployed, Version: 7},
	}, metas)
}

func TestAI_StorageListReleaseMeta_ViaMetadata(t *testing.T) {
	storage, driver := newSecretStorage(t,
		newTestRelease("one", 1, nil),
		newTestRelease("one", 2, nil),
		newTestRelease("two", 1, nil),
	)

	labelSets := versionLabelSets("one", 1, 2)
	labelSets = append(labelSets, versionLabelSets("two", 1)...)
	labelSets = append(labelSets,
		map[string]string{"owner": "helm", "name": "broken", "version": "not-a-number"},
		map[string]string{"owner": "helm", "version": "1"},
	)

	driver.MetadataClient = newMetadataClient(t, labelSets...)
	driver.Namespace = testNamespace

	metas, err := storage.ListReleaseMeta()
	require.NoError(t, err)

	assert.ElementsMatch(t, []helmdriver.ReleaseMeta{
		{Name: "one", Namespace: testNamespace, Version: 1},
		{Name: "one", Namespace: testNamespace, Version: 2},
		{Name: "two", Namespace: testNamespace, Version: 1},
	}, metas, "metadata without a name or with a non-numeric version must be ignored")
}
