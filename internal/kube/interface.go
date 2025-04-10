package kube

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/resource/id"
)

type KubeClienter interface {
	Get(ctx context.Context, resource *id.ResourceID, opts KubeClientGetOptions) (*unstructured.Unstructured, error)
	Create(ctx context.Context, resource *id.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, resource *id.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)
	MergePatch(ctx context.Context, resource *id.ResourceID, patch []byte) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, resource *id.ResourceID, opts KubeClientDeleteOptions) error
}
