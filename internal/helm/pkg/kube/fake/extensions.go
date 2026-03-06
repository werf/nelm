package fake

import (
	"context"

	"github.com/werf/nelm/internal/helm/pkg/kube"
)

func (c *PrintingKubeClient) DeleteNamespace(ctx context.Context, namespace string, opts kube.DeleteOptions) error {
	return nil
}
