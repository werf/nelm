package resrcpatcher

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"nelm.sh/nelm/pkg/resrc"
)

type ResourcePatcher interface {
	Match(ctx context.Context, resourceInfo *ResourceInfo) (matched bool, err error)
	Patch(ctx context.Context, matchedResourceInfo *ResourceInfo) (output *unstructured.Unstructured, err error)
	Type() Type
}

type ResourceInfo struct {
	Obj          *unstructured.Unstructured
	Type         resrc.Type
	ManageableBy resrc.ManageableBy
}

type Type string
