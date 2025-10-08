package fake

import (
	"k8s.io/apimachinery/pkg/api/meta"
	staticfake "k8s.io/client-go/kubernetes/fake"
)

func NewStaticClient(mapper meta.ResettableRESTMapper) *staticfake.Clientset {
	staticClient := staticfake.NewSimpleClientset()
	staticClient.PrependReactor("*", "*", prepareReaction(staticClient.Tracker(), mapper))

	return staticClient
}
