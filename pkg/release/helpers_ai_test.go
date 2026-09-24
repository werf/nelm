//go:build ai_tests

package release

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metadatafake "k8s.io/client-go/metadata/fake"

	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
)

var configMapsGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

func newMetadataClientForGVR(t *testing.T, gvr schema.GroupVersionResource, kind string, labelSets ...map[string]string) *metadatafake.FakeMetadataClient {
	t.Helper()

	scheme := metadatafake.NewTestScheme()
	require.NoError(t, metav1.AddMetaToScheme(scheme))

	client := metadatafake.NewSimpleMetadataClient(scheme)
	resourceClient := client.Resource(gvr).Namespace(testNamespace).(metadatafake.MetadataClient)

	for i, labels := range labelSets {
		obj := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kind},
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

func newTestReleaseWithStatus(name string, version int, status helmreleasecommon.Status) *helmrelease.Release {
	return &helmrelease.Release{
		Name:      name,
		Namespace: testNamespace,
		Version:   version,
		Info:      &helmrelease.Info{Status: status},
	}
}
