package resourcev2

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewRemoteResource(unstruct *unstructured.Unstructured) *RemoteResource {
	return &RemoteResource{
		remoteBaseResource:     newRemoteBaseResource(unstruct),
		helmManageableResource: newHelmManageableResource(unstruct),
		neverDeletableResource: newNeverDeletableResource(unstruct),
		trackableResource:      newTrackableResource(unstruct),
	}
}

type RemoteResource struct {
	*remoteBaseResource
	*helmManageableResource
	*neverDeletableResource
	*trackableResource
}
