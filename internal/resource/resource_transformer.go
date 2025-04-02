package resource

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ResourceTransformerType string

type ResourceTransformer interface {
	Match(ctx context.Context, resourceInfo *ResourceTransformerResourceInfo) (matched bool, err error)
	Transform(ctx context.Context, matchedResourceInfo *ResourceTransformerResourceInfo) (output []*unstructured.Unstructured, err error)
	Type() ResourceTransformerType
}

type ResourceTransformerResourceInfo struct {
	Obj          *unstructured.Unstructured
	Type         Type
	ManageableBy ManageableBy
}
