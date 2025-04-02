package resource

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ ResourceTransformer = (*ResourceListsTransformer)(nil)

const TypeResourceListsTransformer ResourceTransformerType = "resource-lists-transformer"

func NewResourceListsTransformer() *ResourceListsTransformer {
	return &ResourceListsTransformer{}
}

type ResourceListsTransformer struct{}

func (t *ResourceListsTransformer) Match(ctx context.Context, info *ResourceTransformerResourceInfo) (matched bool, err error) {
	switch info.Type {
	case TypeHookResource, TypeGeneralResource:
	default:
		return false, nil
	}

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
