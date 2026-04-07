package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func IsNotFound(err error) bool {
	return err != nil && apierrors.IsNotFound(err)
}

func (c *Client) DeleteNamespace(ctx context.Context, namespace string, opts DeleteOptions) error {
	cs, err := c.Factory.KubernetesClientSet()
	if err != nil {
		return err
	}

	if err := cs.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		return err
	}

	if opts.Wait {
		specs := []*ResourcesWaiterDeleteResourceSpec{
			{ResourceName: namespace, Namespace: "", GroupVersionResource: corev1.SchemeGroupVersion.WithResource("namespaces")},
		}
		if err := c.ResourcesWaiter.WaitUntilDeleted(context.Background(), specs, opts.WaitTimeout); err != nil {
			return fmt.Errorf("waiting until namespace deleted failed: %s", err)
		}
	}

	return nil
}
