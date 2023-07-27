package resourcev2

import (
	"helm.sh/helm/v3/pkg/werf/resourcev2/resourceparts"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewRemoteResource(unstruct *unstructured.Unstructured) *RemoteResource {
	return &RemoteResource{
		RemoteBaseResource:     resourceparts.NewRemoteBaseResource(unstruct),
		HelmManageableResource: resourceparts.NewHelmManageableResource(unstruct),
		NeverDeletableResource: resourceparts.NewNeverDeletableResource(unstruct),
		TrackableResource:      resourceparts.NewTrackableResource(resourceparts.NewTrackableResourceOptions{Unstructured: unstruct}),
	}
}

type RemoteResource struct {
	*resourceparts.RemoteBaseResource
	*resourceparts.HelmManageableResource
	*resourceparts.NeverDeletableResource
	*resourceparts.TrackableResource
}
