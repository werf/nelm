package release

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	v2release "github.com/werf/nelm/v2/pkg/helm/intern/release/v2"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	helmstorage "github.com/werf/nelm/v2/pkg/helm/pkg/storage"
	helmdriver "github.com/werf/nelm/v2/pkg/helm/pkg/storage/driver"
)

func TestAccessorCopyAndSetStatus_V1PreservesDescriptionAndOriginal(t *testing.T) {
	original := &helmrelease.Release{
		Name: "myrelease",
		Info: &helmrelease.Info{
			Status:      helmreleasecommon.StatusDeployed,
			Description: "original description",
		},
	}

	acc, err := helmrel.NewAccessor(original)
	require.NoError(t, err)

	copied, err := acc.Copy()
	require.NoError(t, err)
	copied.SetStatus(helmreleasecommon.StatusFailed)

	copiedRel, ok := copied.Releaser().(*helmrelease.Release)
	require.True(t, ok)
	assert.Equal(t, helmreleasecommon.StatusFailed, copiedRel.Info.Status)
	assert.Equal(t, "original description", copiedRel.Info.Description)

	assert.Equal(t, helmreleasecommon.StatusDeployed, original.Info.Status, "original must not be mutated")
}

func TestAccessorCopyAndSetStatus_V2PreservesDescriptionAndOriginal(t *testing.T) {
	original := &v2release.Release{
		Name: "myrelease",
		Info: &v2release.Info{
			Status:      helmreleasecommon.StatusDeployed,
			Description: "original description",
		},
	}

	acc, err := helmrel.NewAccessor(original)
	require.NoError(t, err)

	copied, err := acc.Copy()
	require.NoError(t, err)
	copied.SetStatus(helmreleasecommon.StatusFailed)

	copiedRel, ok := copied.Releaser().(*v2release.Release)
	require.True(t, ok)
	assert.Equal(t, helmreleasecommon.StatusFailed, copiedRel.Info.Status)
	assert.Equal(t, "original description", copiedRel.Info.Description)

	assert.Equal(t, helmreleasecommon.StatusDeployed, original.Info.Status, "original must not be mutated")
}

func TestReleaseStorage_CreateAndUpdateWriteHelmFormat(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := context.Background()
			s := newTestKubeStorage(t, kind, testNamespace, 0)

			rls := newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusPendingInstall)
			rls.Labels = map[string]string{"custom": "value"}

			acc, err := helmrel.NewAccessor(rls)
			require.NoError(t, err)
			require.NoError(t, s.storage.Create(ctx, acc))

			created, err := s.backend.get(ctx, testNamespace, "sh.helm.release.v1.myrel.v1")
			require.NoError(t, err)

			if kind == kubeStorageKindSecret {
				secret, err := s.client.CoreV1().Secrets(testNamespace).Get(ctx, "sh.helm.release.v1.myrel.v1", metav1.GetOptions{})
				require.NoError(t, err)
				assert.Equal(t, corev1.SecretType("helm.sh/release.v1"), secret.Type)
			}

			assert.Equal(t, "myrel", created.Labels["name"])
			assert.Equal(t, "helm", created.Labels["owner"])
			assert.Equal(t, "pending-install", created.Labels["status"])
			assert.Equal(t, "1", created.Labels["version"])
			assert.Equal(t, "value", created.Labels["custom"])
			assert.NotEmpty(t, created.Labels["createdAt"])

			loaded, err := s.storage.GetRelease(ctx, "myrel", 1)
			require.NoError(t, err)

			loaded.SetStatus(helmreleasecommon.StatusDeployed)
			require.NoError(t, s.storage.Update(ctx, loaded))

			updated, err := s.backend.get(ctx, testNamespace, "sh.helm.release.v1.myrel.v1")
			require.NoError(t, err)
			assert.Equal(t, "deployed", updated.Labels["status"])
			assert.Equal(t, created.Labels["createdAt"], updated.Labels["createdAt"], "update keeps the creation timestamp")
			assert.NotEmpty(t, updated.Labels["modifiedAt"])
			assert.Equal(t, "value", updated.Labels["custom"])

			rlsFromBody, err := decodeRelease(updated.Body)
			require.NoError(t, err)
			assert.Equal(t, helmreleasecommon.StatusDeployed, rlsFromBody.Info.Status, "the status is rewritten in the body too")

			relabeled, err := s.storage.GetRelease(ctx, "myrel", 1)
			require.NoError(t, err)

			relabeledRls, err := ReleaserToV1Release(relabeled.Releaser())
			require.NoError(t, err)

			relabeledRls.Labels = map[string]string{"team": "new"}

			require.NoError(t, s.storage.Update(ctx, relabeled))

			updated, err = s.backend.get(ctx, testNamespace, "sh.helm.release.v1.myrel.v1")
			require.NoError(t, err)
			assert.Equal(t, "new", updated.Labels["team"])
			assert.NotContains(t, updated.Labels, "custom")
		})
	}
}

func TestReleaseStorage_CreateExistingRevision(t *testing.T) {
	s := newMemoryReleaseStorage(t, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed))

	err := s.Create(context.Background(), newTestReleaseAccessor(t, "myrel", 1, helmreleasecommon.StatusDeployed))
	require.ErrorIs(t, err, ErrReleaseExists)
}

func TestReleaseStorage_CreatePrunesLikeHelm(t *testing.T) {
	deployed := helmreleasecommon.StatusDeployed
	superseded := helmreleasecommon.StatusSuperseded
	failed := helmreleasecommon.StatusFailed

	tests := []struct {
		name     string
		statuses []helmreleasecommon.Status
		limit    int
	}{
		{name: "steady history over the limit", statuses: []helmreleasecommon.Status{superseded, superseded, superseded, superseded, deployed}, limit: 3},
		{name: "deployed revision older than failed ones", statuses: []helmreleasecommon.Status{superseded, deployed, failed, failed, failed}, limit: 2},
		{name: "no deployed revision", statuses: []helmreleasecommon.Status{failed, failed, failed, failed}, limit: 3},
		{name: "two deployed revisions", statuses: []helmreleasecommon.Status{superseded, deployed, superseded, deployed}, limit: 2},
		{name: "under the limit", statuses: []helmreleasecommon.Status{superseded, deployed}, limit: 5},
		{name: "limit of one keeps only the deployed revision", statuses: []helmreleasecommon.Status{superseded, deployed, failed}, limit: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rels []*helmrelease.Release
			for i, status := range tt.statuses {
				rels = append(rels, newTestReleaseWithStatus("myrel", i+1, status))
			}

			newRel := newTestReleaseWithStatus("myrel", len(rels)+1, helmreleasecommon.StatusPendingUpgrade)

			helmDriver := helmdriver.NewMemory()
			helmDriver.SetNamespace(testNamespace)
			helmStorage := helmstorage.Init(helmDriver)
			helmStorage.MaxHistory = tt.limit

			for _, rls := range rels {
				require.NoError(t, helmDriver.Create(fmt.Sprintf("sh.helm.release.v1.%s.v%d", rls.Name, rls.Version), rls))
			}

			require.NoError(t, helmStorage.Create(newRel))

			helmHistory, err := helmStorage.History("myrel")
			require.NoError(t, err)

			var helmVersions []int
			for _, releaser := range helmHistory {
				acc, err := helmrel.NewAccessor(releaser)
				require.NoError(t, err)

				helmVersions = append(helmVersions, acc.Version())
			}

			slices.Sort(helmVersions)

			storage := newMemoryReleaseStorage(t, rels...)
			storage.historyLimit = tt.limit

			newAcc, err := helmrel.NewAccessor(newTestReleaseWithStatus("myrel", len(rels)+1, helmreleasecommon.StatusPendingUpgrade))
			require.NoError(t, err)
			require.NoError(t, storage.Create(context.Background(), newAcc))

			revisions, err := storage.Revisions(context.Background(), "myrel")
			require.NoError(t, err)

			var versions []int
			for _, revision := range revisions {
				versions = append(versions, revision.Version)
			}

			assert.Equal(t, helmVersions, versions)
		})
	}
}

func TestReleaseStorage_CreatePrunesWithoutReadingBodies(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 3)

	putTestKubeObject(t, s, newUndecodableTestStoredObject(t, testNamespace, "myrel", 1, helmreleasecommon.StatusSuperseded))
	putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", 2, helmreleasecommon.StatusSuperseded))
	putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", 3, helmreleasecommon.StatusSuperseded))
	putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", 4, helmreleasecommon.StatusDeployed))

	require.NoError(t, s.storage.Create(context.Background(), newTestReleaseAccessor(t, "myrel", 5, helmreleasecommon.StatusPendingUpgrade)))

	actions := s.client.Actions()
	assert.Equal(t, 0, countActions(actions, "get"), "pruning must not fetch release bodies")
	assert.Equal(t, 0, countActions(actions, "list"), "pruning must not list release bodies")
	assert.Equal(t, 2, countActions(actions, "delete"))
	assert.Equal(t, 1, countActions(actions, "create"))

	var names []string
	for _, secret := range listTestKubeSecrets(t, s, testNamespace) {
		names = append(names, secret.Name)
	}

	assert.ElementsMatch(t, []string{"sh.helm.release.v1.myrel.v3", "sh.helm.release.v1.myrel.v4", "sh.helm.release.v1.myrel.v5"}, names,
		"an undecodable revision is pruned like any other")
}

func TestReleaseStorage_DeleteMissingRevision(t *testing.T) {
	s := newMemoryReleaseStorage(t)

	require.ErrorIs(t, s.Delete(context.Background(), "myrel", 1), ErrReleaseNotFound)
}

func TestReleaseStorage_GetReleaseKeepsStorageLabels(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)

	rls := newTestReleaseWithStatus("myrel", 2, helmreleasecommon.StatusDeployed)
	rls.Labels = map[string]string{"moduleChecksum": "bbb"}
	putTestKubeRelease(t, s, rls)

	rel, err := s.storage.GetRelease(context.Background(), "myrel", 2)
	require.NoError(t, err)
	assert.Equal(t, "bbb", rel.Labels()["moduleChecksum"])
	assert.Equal(t, "helm", rel.Labels()["owner"])
	assert.Equal(t, "2", rel.Labels()["version"])
}

func TestReleaseStorage_GetReleaseLatestFetchesOneBody(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	for _, version := range []int{1, 2, 9, 10, 11} {
		putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", version, helmreleasecommon.StatusSuperseded))
	}

	putTestKubeRelease(t, s, newTestReleaseWithStatus("otherrel", 99, helmreleasecommon.StatusDeployed))

	rel, err := s.storage.GetRelease(context.Background(), "myrel", 0)
	require.NoError(t, err)
	assert.Equal(t, 11, rel.Version(), "the latest revision is the numeric maximum")

	assert.Equal(t, 1, countActions(s.client.Actions(), "get"))
	assert.Equal(t, 0, countActions(s.client.Actions(), "list"))
}

func TestReleaseStorage_GetReleaseLatestRelistsWhenLatestIsRemoved(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	for _, version := range []int{1, 2, 4} {
		putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", version, helmreleasecommon.StatusSuperseded))
	}

	listings := [][]int{{1, 2, 3}, {1, 2, 4}}

	var calls int
	s.metadataClient.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		require.Less(t, calls, len(listings))

		list := &metav1.List{}
		for _, version := range listings[calls] {
			list.Items = append(list.Items, runtime.RawExtension{Object: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      storageKey("myrel", version),
					Labels:    map[string]string{"owner": "helm", "name": "myrel", "version": strconv.Itoa(version), "status": "superseded"},
				},
			}})
		}

		calls++

		return true, list, nil
	})

	rel, err := s.storage.GetRelease(context.Background(), "myrel", 0)
	require.NoError(t, err)
	assert.Equal(t, 4, rel.Version())
	assert.Equal(t, 2, calls)
}

func TestReleaseStorage_GetReleaseLatestSkipsUnparseableVersion(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	for _, version := range []int{1, 2, 11} {
		putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", version, helmreleasecommon.StatusSuperseded))
	}

	putTestKubeObject(t, s, &storedObject{Namespace: testNamespace, Key: "broken", Labels: map[string]string{"owner": "helm", "name": "myrel", "version": "not-a-number"}})
	putTestKubeRelease(t, s, newTestReleaseWithStatus("otherrel", 99, helmreleasecommon.StatusDeployed))

	rel, err := s.storage.GetRelease(context.Background(), "myrel", 0)
	require.NoError(t, err)
	assert.Equal(t, 11, rel.Version())
}

func TestReleaseStorage_GetReleaseNotFound(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)

	_, err := s.storage.GetRelease(context.Background(), "absent", 0)
	require.ErrorIs(t, err, ErrReleaseNotFound)

	_, err = s.storage.GetRelease(context.Background(), "absent", 3)
	require.ErrorIs(t, err, ErrReleaseNotFound)
}

func TestReleaseStorage_GetReleaseUndecodable(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	putTestKubeObject(t, s, newUndecodableTestStoredObject(t, testNamespace, "myrel", 1, helmreleasecommon.StatusDeployed))

	_, err := s.storage.GetRelease(context.Background(), "myrel", 0)
	require.ErrorIs(t, err, ErrReleaseUndecodable)
	require.NotErrorIs(t, err, ErrReleaseNotFound)
	assert.Contains(t, err.Error(), `"sh.helm.release.v1.myrel.v1"`)
	assert.Contains(t, err.Error(), `"test-ns"`)
}

func TestReleaseStorage_LatestRevisionsPaginates(t *testing.T) {
	pages := map[string]*metav1.PartialObjectMetadataList{
		"": newTestMetadataPage("page-1",
			map[string]string{"owner": "helm", "name": "a", "version": "1", "status": "superseded"}),
		"page-1": newTestMetadataPage("page-2"),
		"page-2": newTestMetadataPage("",
			map[string]string{"owner": "helm", "name": "a", "version": "2", "status": "deployed"},
			map[string]string{"owner": "helm", "name": "b", "version": "1", "status": "failed"}),
	}
	client := &pagedMetadataClient{pages: pages}
	storage := newReleaseStorage(testNamespace, newKubeStorageBackend(kubeStorageKindSecret, k8sfake.NewClientset(), client), 0)

	revisions, err := storage.LatestRevisions(context.Background(), LatestRevisionsOptions{})
	require.NoError(t, err)
	assert.Equal(t, []Revision{
		{Name: "a", Namespace: testNamespace, Status: "deployed", Version: 2},
		{Name: "b", Namespace: testNamespace, Status: "failed", Version: 1},
	}, revisions)
	assert.Equal(t, []string{"", "page-1", "page-2"}, lo.Map(client.requests, func(opts metav1.ListOptions, _ int) string { return opts.Continue }))

	for _, opts := range client.requests {
		assert.Equal(t, int64(kubeStoragePageSize), opts.Limit)
		assert.Equal(t, "owner=helm", opts.LabelSelector)
	}
}

func TestReleaseStorage_LatestRevisionsSameNameInManyNamespaces(t *testing.T) {
	ctx := context.Background()
	s := newTestKubeStorage(t, kubeStorageKindSecret, "", 0)

	const namespaces = 20
	for i := range namespaces {
		for version := 1; version <= 3; version++ {
			putTestKubeRelease(t, s, newTestRelease(fmt.Sprintf("ns-%02d", i), "myrel", version, helmreleasecommon.StatusDeployed))
		}
	}

	revisions, err := s.storage.LatestRevisions(ctx, LatestRevisionsOptions{})
	require.NoError(t, err)
	require.Len(t, revisions, namespaces)

	assert.Equal(t, 1, countActions(s.metadataClient.Actions(), "list"))
	assert.Empty(t, s.client.Actions(), "listing reads no release bodies")

	for i, revision := range revisions {
		assert.Equal(t, fmt.Sprintf("ns-%02d", i), revision.Namespace)
		assert.Equal(t, 3, revision.Version)

		rel, err := s.storage.LoadRevision(ctx, revision)
		require.NoError(t, err)
		assert.Equal(t, revision.Namespace, rel.Namespace())
		assert.Equal(t, 3, rel.Version())
	}

	assert.Equal(t, namespaces, countActions(s.client.Actions(), "get"), "one body is fetched per release")
	assert.Equal(t, 0, countActions(s.client.Actions(), "list"))
}

func TestReleaseStorage_LatestRevisionsSkipsObjectsThatAreNotRevisions(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed))

	for i, labels := range []map[string]string{
		{"owner": "helm", "version": "5"},
		{"owner": "helm", "name": "myrel"},
		{"owner": "helm", "name": "myrel", "version": "garbage"},
	} {
		putTestKubeObject(t, s, &storedObject{Namespace: testNamespace, Key: fmt.Sprintf("stray-%d", i), Labels: labels})
	}

	revisions, err := s.storage.LatestRevisions(context.Background(), LatestRevisionsOptions{})
	require.NoError(t, err)
	assert.Equal(t, []Revision{{Name: "myrel", Namespace: testNamespace, Status: "deployed", Version: 1}}, revisions)
}

func TestReleaseStorage_ListRevisionSummariesPaginates(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)

	pages := []struct {
		next    string
		version int
	}{
		{next: "page-1", version: 1},
		{version: 2},
	}
	pageByContinue := map[string]int{"": 0, "page-1": 1}

	var calls int
	s.client.PrependReactor("list", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		calls++
		require.LessOrEqual(t, calls, len(pages))

		opts := action.(k8stesting.ListActionImpl).GetListOptions()
		assert.Equal(t, int64(kubeStoragePageSize), opts.Limit)

		page, found := pageByContinue[opts.Continue]
		require.True(t, found, "unexpected continue token %q", opts.Continue)

		obj := newTestStoredObject(t, newTestReleaseWithStatus("myrel", pages[page].version, helmreleasecommon.StatusSuperseded))

		list := &corev1.SecretList{Items: []corev1.Secret{*newKubeSecret(obj)}}
		list.Continue = pages[page].next

		return true, list, nil
	})

	summaries, err := s.storage.ListRevisionSummaries(context.Background(), "myrel", ListRevisionSummariesOptions{})
	require.NoError(t, err)

	assert.Equal(t, []int{1, 2}, summaryVersions(t, summaries))
	assert.Equal(t, 2, calls)
}

func TestReleaseStorage_ListRevisionSummariesReportsUndecodableRevision(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusSuperseded))
	putTestKubeObject(t, s, newUndecodableTestStoredObject(t, testNamespace, "myrel", 2, helmreleasecommon.StatusDeployed))
	putTestKubeRelease(t, s, newTestReleaseWithStatus("otherrel", 1, helmreleasecommon.StatusDeployed))

	summaries, err := s.storage.ListRevisionSummaries(context.Background(), "myrel", ListRevisionSummariesOptions{})
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	require.NoError(t, summaries[0].DecodeErr)
	assert.NotNil(t, summaries[0].Summary)
	require.ErrorIs(t, summaries[1].DecodeErr, ErrReleaseUndecodable)
	assert.Nil(t, summaries[1].Summary)
	assert.Equal(t, "deployed", summaries[1].Revision.Status)
	assert.Equal(t, 1, countActions(s.client.Actions(), "list"))
}

func TestReleaseStorage_ListRevisionSummariesSortsByVersion(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	for _, version := range []int{2, 10, 1} {
		putTestKubeObject(t, s, newTestStoredObject(t, newTestRelease(testNamespace, "myrel", version, helmreleasecommon.StatusSuperseded)))
	}

	summaries, err := s.storage.ListRevisionSummaries(context.Background(), "myrel", ListRevisionSummariesOptions{})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 10}, summaryVersions(t, summaries))
}

func TestReleaseStorage_ListRevisionSummariesWithLimitInMemory(t *testing.T) {
	var rels []*helmrelease.Release
	for _, version := range []int{1, 2, 9, 10} {
		rels = append(rels, newTestReleaseWithStatus("myrel", version, helmreleasecommon.StatusSuperseded))
	}

	summaries, err := newMemoryReleaseStorage(t, rels...).ListRevisionSummaries(context.Background(), "myrel", ListRevisionSummariesOptions{Limit: 2})
	require.NoError(t, err)

	assert.Equal(t, []int{9, 10}, summaryVersions(t, summaries))
}

func TestReleaseStorage_ListRevisionSummariesWithLimitReadsOnlyNewestBodies(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	for _, version := range []int{1, 2, 9, 10} {
		putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", version, helmreleasecommon.StatusSuperseded))
	}

	putTestKubeObject(t, s, newUndecodableTestStoredObject(t, testNamespace, "myrel", 11, helmreleasecommon.StatusDeployed))
	putTestKubeObject(t, s, &storedObject{Namespace: testNamespace, Key: "broken", Labels: map[string]string{"owner": "helm", "name": "myrel", "version": "x"}})

	summaries, err := s.storage.ListRevisionSummaries(context.Background(), "myrel", ListRevisionSummariesOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, 10, summaries[0].Revision.Version)
	require.NoError(t, summaries[0].DecodeErr)
	assert.Equal(t, 11, summaries[1].Revision.Version)
	require.ErrorIs(t, summaries[1].DecodeErr, ErrReleaseUndecodable)
	assert.Equal(t, 0, countActions(s.client.Actions(), "get"))
	require.Equal(t, 1, countActions(s.client.Actions(), "list"))

	for _, action := range s.client.Actions() {
		if listAction, ok := action.(k8stesting.ListActionImpl); ok {
			assert.Equal(t, "name=myrel,owner=helm,version in (10,11)", listAction.GetListOptions().LabelSelector)
		}
	}
}

func TestReleaseStorage_ListRevisionSummariesWithLimitSkipsRemovedRevision(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	for _, version := range []int{1, 2, 3} {
		putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", version, helmreleasecommon.StatusSuperseded))
	}

	require.NoError(t, s.backend.delete(context.Background(), testNamespace, storageKey("myrel", 2)))

	summaries, err := s.storage.ListRevisionSummaries(context.Background(), "myrel", ListRevisionSummariesOptions{Limit: 2})
	require.NoError(t, err)

	assert.Equal(t, []int{3}, summaryVersions(t, summaries))
}

func TestReleaseStorage_NamedReadsRequireName(t *testing.T) {
	ctx := context.Background()
	s := newTestKubeStorage(t, kubeStorageKindSecret, testNamespace, 0)
	putTestKubeRelease(t, s, newTestReleaseWithStatus("other", 7, helmreleasecommon.StatusDeployed))

	_, err := s.storage.GetRelease(ctx, "", 0)
	require.Error(t, err)

	_, err = s.storage.GetRelease(ctx, "", 7)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrReleaseNotFound)

	_, err = s.storage.Revisions(ctx, "")
	require.Error(t, err)

	_, err = s.storage.ListRevisionSummaries(ctx, "", ListRevisionSummariesOptions{})
	require.Error(t, err)
}

func TestReleaseStorage_NamespacedOperationsRequireNamespace(t *testing.T) {
	ctx := context.Background()
	s := newTestKubeStorage(t, kubeStorageKindSecret, "", 0)
	acc := newTestReleaseAccessor(t, "myrel", 1, helmreleasecommon.StatusDeployed)

	require.Error(t, s.storage.Create(ctx, acc))
	require.Error(t, s.storage.Update(ctx, acc))
	require.Error(t, s.storage.Delete(ctx, "myrel", 1))
	require.Error(t, s.storage.UpdateLabels(ctx, "myrel", 1, map[string]string{"a": "b"}))

	_, err := s.storage.GetRelease(ctx, "myrel", 1)
	require.Error(t, err)

	_, err = s.storage.Revisions(ctx, "myrel")
	require.Error(t, err)

	_, err = s.storage.ListRevisionSummaries(ctx, "myrel", ListRevisionSummariesOptions{})
	require.Error(t, err)
}

func TestReleaseStorage_RevisionsAreSortedAndUnparseableVersionIsAnError(t *testing.T) {
	ctx := context.Background()
	s := newTestKubeStorage(t, kubeStorageKindConfigMap, testNamespace, 0)

	for _, version := range []int{10, 2, 1} {
		putTestKubeRelease(t, s, newTestReleaseWithStatus("myrel", version, helmreleasecommon.StatusSuperseded))
	}

	putTestKubeObject(t, s, &storedObject{Namespace: testNamespace, Key: "stray", Labels: map[string]string{"owner": "helm", "name": "myrel"}})

	revisions, err := s.storage.Revisions(ctx, "myrel")
	require.NoError(t, err)

	var versions []int
	for _, revision := range revisions {
		versions = append(versions, revision.Version)
	}

	assert.Equal(t, []int{1, 2, 10}, versions)
	assert.Empty(t, s.client.Actions(), "revisions are read from metadata only")

	putTestKubeObject(t, s, &storedObject{Namespace: testNamespace, Key: "broken", Labels: map[string]string{"owner": "helm", "name": "myrel", "version": "x"}})

	_, err = s.storage.Revisions(ctx, "myrel")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"broken"`)
}

func TestReleaseStorage_UpdateLabelsKeepsSystemLabels(t *testing.T) {
	ctx := context.Background()
	s := newMemoryReleaseStorage(t, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed))

	require.NoError(t, s.UpdateLabels(ctx, "myrel", 1, map[string]string{"custom": "value", "status": "failed"}))

	rel, err := s.GetRelease(ctx, "myrel", 1)
	require.NoError(t, err)
	assert.Equal(t, "value", rel.Labels()["custom"])
	assert.Equal(t, "deployed", rel.Labels()["status"])

	require.ErrorIs(t, s.UpdateLabels(ctx, "myrel", 2, map[string]string{"custom": "value"}), ErrReleaseNotFound)
}
