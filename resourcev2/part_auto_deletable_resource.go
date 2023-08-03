package resourcev2

import (
	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/werf/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newAutoDeletableResource(unstruct *unstructured.Unstructured) *autoDeletableResource {
	return &autoDeletableResource{
		unstructured: unstruct,
	}
}

type autoDeletableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *autoDeletableResource) Validate() error {
	if err := validateDeletePolicyAnnotations(r.unstructured.GetAnnotations()); err != nil {
		return err
	}

	return nil
}

func (r *autoDeletableResource) ShouldCleanupWhenSucceeded() bool {
	deletePolicies := getDeletePolicies(r.unstructured.GetAnnotations())

	return lo.Contains(deletePolicies, common.DeletePolicySucceeded)
}

func (r *autoDeletableResource) ShouldCleanupWhenFailed() bool {
	deletePolicies := getDeletePolicies(r.unstructured.GetAnnotations())

	return lo.Contains(deletePolicies, common.DeletePolicyFailed)
}
