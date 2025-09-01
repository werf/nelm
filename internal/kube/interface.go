package kube

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/resource/id"
)

type KubeClienter interface {
	Get(ctx context.Context, meta *id.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error)
	Create(ctx context.Context, spec *id.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, spec *id.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)
	MergePatch(ctx context.Context, meta *id.ResourceMeta, patch []byte) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, meta *id.ResourceMeta, opts KubeClientDeleteOptions) error
}
