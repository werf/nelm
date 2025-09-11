package resource

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/common"
)

type ResourcePatcher interface {
	Match(ctx context.Context, resourceInfo *ResourcePatcherResourceInfo) (matched bool, err error)
	Patch(ctx context.Context, matchedResourceInfo *ResourcePatcherResourceInfo) (output *unstructured.Unstructured, err error)
	Type() ResourcePatcherType
}

type ResourcePatcherResourceInfo struct {
	Obj       *unstructured.Unstructured
	Ownership common.Ownership
}

type ResourcePatcherType string
