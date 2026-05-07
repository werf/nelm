package spec

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const TypeResourceListsTransformer ResourceTransformerType = "resource-lists-transformer"

var _ ResourceTransformer = (*ResourceListsTransformer)(nil)

type ResourceTransformerType string

type ResourceTransformer interface {
	Match(ctx context.Context, resourceInfo *ResourceTransformerResourceInfo) (matched bool, err error)
	Transform(ctx context.Context, matchedResourceInfo *ResourceTransformerResourceInfo) (output []*unstructured.Unstructured, err error)
	Type() ResourceTransformerType
}

type ResourceTransformerResourceInfo struct {
	Obj *unstructured.Unstructured
}

type ResourceListsTransformer struct{}

func NewResourceListsTransformer() *ResourceListsTransformer {
	return &ResourceListsTransformer{}
}

func (t *ResourceListsTransformer) Match(ctx context.Context, info *ResourceTransformerResourceInfo) (matched bool, err error) {
	return info.Obj.IsList(), nil
}

func (t *ResourceListsTransformer) Transform(ctx context.Context, info *ResourceTransformerResourceInfo) ([]*unstructured.Unstructured, error) {
	var result []*unstructured.Unstructured

	if err := info.Obj.EachListItem(
		func(obj runtime.Object) error {
			result = append(result, obj.(*unstructured.Unstructured))

			return nil
		},
	); err != nil {
		return nil, fmt.Errorf("error iterating over list items: %w", err)
	}

	return result, nil
}

func (t *ResourceListsTransformer) Type() ResourceTransformerType {
	return TypeResourceListsTransformer
}
