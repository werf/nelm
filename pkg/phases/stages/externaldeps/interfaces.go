package externaldeps

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GVKBuilder interface {
	BuildFromResource(resource string) (*schema.GroupVersionKind, error)
}
