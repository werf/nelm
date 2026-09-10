//go:build ai_tests

package release //nolint:testpackage

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	helmdriver "github.com/werf/nelm/pkg/helm/pkg/storage/driver"
)

var _ helmdriver.Driver = (*plainDriver)(nil)

// plainDriver forwards to the memory driver without exposing ListLatestReleases,
// so that Storage has to take its fallback path. The methods are forwarded
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

// pagingSecretClient serves a secret list page by page, honoring Limit and
// Continue, which the fake clientset ignores.
type pagingSecretClient struct {
	corev1.SecretInterface

	items []v1.Secret
	limit int64
	pages int
}

func (c *pagingSecretClient) List(_ context.Context, opts metav1.ListOptions) (*v1.SecretList, error) {
	c.pages++
	c.limit = opts.Limit

	from := 0
	if opts.Continue != "" {
		if _, err := fmt.Sscanf(opts.Continue, "offset-%d", &from); err != nil {
			return nil, fmt.Errorf("parse continue token %q: %w", opts.Continue, err)
		}
	}

	const pageSize = 2

	to := min(from+pageSize, len(c.items))

	list := &v1.SecretList{Items: c.items[from:to]}
	if to < len(c.items) {
		list.Continue = fmt.Sprintf("offset-%d", to)
	}

	return list, nil
}

// namespaceScopedSecretClient applies metadata.namespace field selectors, which
// the fake clientset ignores, and counts the objects it serves.
type namespaceScopedSecretClient struct {
	corev1.SecretInterface

	served int
}

func (c *namespaceScopedSecretClient) List(ctx context.Context, opts metav1.ListOptions) (*v1.SecretList, error) {
	list, err := c.SecretInterface.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	if namespace := strings.TrimPrefix(opts.FieldSelector, "metadata.namespace="); namespace != opts.FieldSelector {
		list.Items = lo.Filter(list.Items, func(item v1.Secret, _ int) bool {
			return item.Namespace == namespace
		})
	}

	c.served += len(list.Items)

	return list, nil
}

// namespaceScopedConfigMapClient applies metadata.namespace field selectors, which
// the fake clientset ignores, and counts the objects it serves.
type namespaceScopedConfigMapClient struct {
	corev1.ConfigMapInterface

	served int
}

func (c *namespaceScopedConfigMapClient) List(ctx context.Context, opts metav1.ListOptions) (*v1.ConfigMapList, error) {
	list, err := c.ConfigMapInterface.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list configmaps: %w", err)
	}

	if namespace := strings.TrimPrefix(opts.FieldSelector, "metadata.namespace="); namespace != opts.FieldSelector {
		list.Items = lo.Filter(list.Items, func(item v1.ConfigMap, _ int) bool {
			return item.Namespace == namespace
		})
	}

	c.served += len(list.Items)

	return list, nil
}

func TestAI_StorageListLatestReleases_ConfigMapCorruptLatestRevisionDoesNotFanOut(t *testing.T) {
	const (
		namespaces = 10
		revisions  = 8
	)

	clientset := k8sfake.NewSimpleClientset()

	for i := 0; i < namespaces; i++ {
		namespace := fmt.Sprintf("ns-%d", i)
		configMaps := clientset.CoreV1().ConfigMaps(namespace)
		storage := helmstorage.Init(helmdriver.NewConfigMaps(configMaps))

		for v := 1; v <= revisions; v++ {
			rel := newTestRelease("myapp", v, nil)
			rel.Namespace = namespace
			require.NoError(t, storage.Create(rel))
		}

		configMap, err := configMaps.Get(context.Background(), fmt.Sprintf("sh.helm.release.v1.myapp.v%d", revisions), metav1.GetOptions{})
		require.NoError(t, err)

		configMap.Data["release"] = "corrupt"
		_, err = configMaps.Update(context.Background(), configMap, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	scoped := &namespaceScopedConfigMapClient{ConfigMapInterface: clientset.CoreV1().ConfigMaps("")}
	listStorage := helmstorage.Init(helmdriver.NewConfigMaps(scoped))

	rels, err := listStorage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	byNamespace := map[string]int{}
	for _, rel := range rels {
		byNamespace[rel.Namespace] = rel.Version
	}

	require.Len(t, byNamespace, namespaces)

	for namespace, version := range byNamespace {
		assert.Equal(t, revisions-1, version, "namespace %s must fall back to its own newest decodable revision", namespace)
	}

	assert.LessOrEqual(t, scoped.served, namespaces*revisions*2,
		"recovering from a corrupt revision must not re-list the whole cross-namespace history of the release name once per namespace")
}

func TestAI_StorageListLatestReleases_ConfigMapDriver(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	storage := helmstorage.Init(helmdriver.NewConfigMaps(clientset.CoreV1().ConfigMaps(testNamespace)))
	require.NoError(t, storage.Create(newTestRelease("one", 1, nil)))
	require.NoError(t, storage.Create(newTestRelease("one", 2, nil)))
	require.NoError(t, storage.Create(newTestRelease("two", 5, nil)))

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"one": 2, "two": 5}, revisionsByName(rels))
}

func TestAI_StorageListLatestReleases_ConfigMapFallsBackFromCorruptLatestRevision(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	configMaps := clientset.CoreV1().ConfigMaps(testNamespace)
	storage := helmstorage.Init(helmdriver.NewConfigMaps(configMaps))

	for version := 1; version <= 3; version++ {
		require.NoError(t, storage.Create(newTestRelease("one", version, nil)))
	}

	for _, version := range []int{2, 3} {
		configMap, err := configMaps.Get(context.Background(), fmt.Sprintf("sh.helm.release.v1.one.v%d", version), metav1.GetOptions{})
		require.NoError(t, err)

		configMap.Data["release"] = "corrupt"
		_, err = configMaps.Update(context.Background(), configMap, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"one": 1}, revisionsByName(rels))
}

func TestAI_StorageListLatestReleases_CorruptLatestRevisionDoesNotFanOut(t *testing.T) {
	const (
		namespaces = 10
		revisions  = 8
	)

	clientset := k8sfake.NewSimpleClientset()

	for i := 0; i < namespaces; i++ {
		namespace := fmt.Sprintf("ns-%d", i)
		secrets := clientset.CoreV1().Secrets(namespace)
		storage := helmstorage.Init(helmdriver.NewSecrets(secrets))

		for v := 1; v <= revisions; v++ {
			rel := newTestRelease("myapp", v, nil)
			rel.Namespace = namespace
			require.NoError(t, storage.Create(rel))
		}

		secret, err := secrets.Get(context.Background(), fmt.Sprintf("sh.helm.release.v1.myapp.v%d", revisions), metav1.GetOptions{})
		require.NoError(t, err)

		secret.Data["release"] = []byte("corrupt")
		_, err = secrets.Update(context.Background(), secret, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	scoped := &namespaceScopedSecretClient{SecretInterface: clientset.CoreV1().Secrets("")}
	listStorage := helmstorage.Init(helmdriver.NewSecrets(scoped))

	rels, err := listStorage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	byNamespace := map[string]int{}
	for _, rel := range rels {
		byNamespace[rel.Namespace] = rel.Version
	}

	require.Len(t, byNamespace, namespaces)

	for namespace, version := range byNamespace {
		assert.Equal(t, revisions-1, version, "namespace %s must fall back to its own newest decodable revision", namespace)
	}

	assert.LessOrEqual(t, scoped.served, namespaces*revisions*2,
		"recovering from a corrupt revision must not re-list the whole cross-namespace history of the release name once per namespace")
}

func TestAI_StorageListLatestReleases_Empty(t *testing.T) {
	storage, _ := newSecretStorage(t)

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rels, "an empty storage is not an error for a listing")
}

func TestAI_StorageListLatestReleases_FallbackForDriverWithoutCapability(t *testing.T) {
	mem := helmdriver.NewMemory()
	mem.SetNamespace(testNamespace)

	storage := helmstorage.Init(&plainDriver{inner: mem})
	require.NoError(t, storage.Create(newTestRelease("one", 1, nil)))
	require.NoError(t, storage.Create(newTestRelease("one", 2, nil)))
	require.NoError(t, storage.Create(newTestRelease("two", 7, nil)))

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"one": 2, "two": 7}, revisionsByName(rels))
}

func TestAI_StorageListLatestReleases_FallbackForEmptyDriverWithoutCapability(t *testing.T) {
	mem := helmdriver.NewMemory()
	mem.SetNamespace(testNamespace)

	storage := helmstorage.Init(&plainDriver{inner: mem})

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rels, "ErrReleaseNotFound from the driver means an empty listing, not a failure")
}

func TestAI_StorageListLatestReleases_IgnoresObjectsWithoutReleaseLabels(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	storage := helmstorage.Init(helmdriver.NewSecrets(clientset.CoreV1().Secrets(testNamespace)))
	require.NoError(t, storage.Create(newTestRelease("one", 3, nil)))

	for name, labels := range map[string]map[string]string{
		"no-name":         {"owner": "helm", "version": "9"},
		"broken-version":  {"owner": "helm", "name": "one", "version": "not-a-number"},
		"missing-version": {"owner": "helm", "name": "one"},
	} {
		_, err := clientset.CoreV1().Secrets(testNamespace).Create(context.Background(), &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: name, Labels: labels},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"one": 3}, revisionsByName(rels),
		"objects without a name or with a non-numeric version must be ignored, not crash the listing")
}

func TestAI_StorageListLatestReleases_NumericMaxRevision(t *testing.T) {
	storage, _ := newSecretStorage(t,
		newTestRelease("one", 1, nil),
		newTestRelease("one", 9, nil),
		newTestRelease("one", 11, nil),
		newTestRelease("one", 2, nil),
	)

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"one": 11}, revisionsByName(rels),
		"the revision must be compared numerically, not lexicographically")
}

func TestAI_StorageListLatestReleases_Paginates(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	storage := helmstorage.Init(helmdriver.NewSecrets(clientset.CoreV1().Secrets(testNamespace)))
	for name, revisions := range map[string]int{"one": 3, "two": 4, "three": 2} {
		for v := 1; v <= revisions; v++ {
			require.NoError(t, storage.Create(newTestRelease(name, v, nil)))
		}
	}

	stored, err := clientset.CoreV1().Secrets(testNamespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, stored.Items, 9)

	paging := &pagingSecretClient{SecretInterface: clientset.CoreV1().Secrets(testNamespace), items: stored.Items}
	pagingStorage := helmstorage.Init(helmdriver.NewSecrets(paging))

	rels, err := pagingStorage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"one": 3, "two": 4, "three": 2}, revisionsByName(rels),
		"the last revision of every release must survive paging")
	assert.Equal(t, 5, paging.pages, "9 items served 2 at a time must be fetched as 5 pages")
	assert.NotZero(t, paging.limit, "the driver must ask the API server for pages, not for everything at once")
}

func TestAI_StorageListLatestReleases_SameNameInManyNamespacesDoesNotFanOut(t *testing.T) {
	const (
		namespaces = 20
		revisions  = 5
	)

	clientset := k8sfake.NewSimpleClientset()

	for i := 0; i < namespaces; i++ {
		namespace := fmt.Sprintf("ns-%d", i)
		storage := helmstorage.Init(helmdriver.NewSecrets(clientset.CoreV1().Secrets(namespace)))

		for v := 1; v <= revisions; v++ {
			rel := newTestRelease("myapp", v, nil)
			rel.Namespace = namespace
			require.NoError(t, storage.Create(rel))
		}
	}

	listStorage := helmstorage.Init(helmdriver.NewSecrets(clientset.CoreV1().Secrets("")))

	clientset.ClearActions()

	rels, err := listStorage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	lists := 0
	for _, action := range clientset.Actions() {
		if action.GetVerb() == "list" {
			lists++
		}
	}

	byNamespace := map[string]int{}
	for _, rel := range rels {
		byNamespace[rel.Namespace] = rel.Version
	}

	require.Len(t, byNamespace, namespaces, "the same release name in different namespaces must stay separate")

	for namespace, version := range byNamespace {
		assert.Equal(t, revisions, version, "namespace %s must report its own last revision", namespace)
	}

	assert.Equal(t, 1, lists,
		"the listing must not issue a request per release: that is quadratic when one name is deployed to many namespaces")
}

func TestAI_StorageListLatestReleases_SecretFallsBackFromCorruptLatestRevision(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	secrets := clientset.CoreV1().Secrets(testNamespace)
	storage := helmstorage.Init(helmdriver.NewSecrets(secrets))

	for version := 1; version <= 3; version++ {
		require.NoError(t, storage.Create(newTestRelease("one", version, nil)))
	}

	for _, version := range []int{2, 3} {
		secret, err := secrets.Get(context.Background(), fmt.Sprintf("sh.helm.release.v1.one.v%d", version), metav1.GetOptions{})
		require.NoError(t, err)

		secret.Data["release"] = []byte("corrupt")
		_, err = secrets.Update(context.Background(), secret, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"one": 1}, revisionsByName(rels))
}

func TestAI_StorageListLatestReleases_SecretOmitsReleaseWhenEveryRevisionIsCorrupt(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	secrets := clientset.CoreV1().Secrets(testNamespace)
	storage := helmstorage.Init(helmdriver.NewSecrets(secrets))
	require.NoError(t, storage.Create(newTestRelease("one", 1, nil)))

	secret, err := secrets.Get(context.Background(), "sh.helm.release.v1.one.v1", metav1.GetOptions{})
	require.NoError(t, err)

	secret.Data["release"] = []byte("corrupt")
	_, err = secrets.Update(context.Background(), secret, metav1.UpdateOptions{})
	require.NoError(t, err)

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rels)
}

func TestAI_StorageListLatestReleases_TakesNamespaceFromObjectWhenBodyHasNone(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	storage := helmstorage.Init(helmdriver.NewSecrets(clientset.CoreV1().Secrets(testNamespace)))

	// Releases stored by ancient Helm versions carry no namespace in the body.
	ancient := newTestRelease("legacy", 4, nil)
	ancient.Namespace = ""
	require.NoError(t, storage.Create(ancient))

	rels, err := storage.ListLatestReleases(context.Background())
	require.NoError(t, err)

	require.Len(t, rels, 1)
	assert.Equal(t, testNamespace, rels[0].Namespace)
}

func revisionsByName(rels []*helmrelease.Release) map[string]int {
	byName := map[string]int{}
	for _, rel := range rels {
		byName[rel.Name] = rel.Version
	}

	return byName
}
