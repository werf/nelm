package resourceparts

import (
	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/werf/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewAutoDeletableResource(unstruct *unstructured.Unstructured) *AutoDeletableResource {
	return &AutoDeletableResource{
		unstructured: unstruct,
	}
}

type AutoDeletableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *AutoDeletableResource) Validate() error {
	if err := validateDeletePolicyAnnotations(r.unstructured.GetAnnotations()); err != nil {
		return err
	}

	return nil
}

func (r *AutoDeletableResource) ShouldCleanupWhenSucceeded() bool {
	deletePolicies := getDeletePolicies(r.unstructured.GetAnnotations())

	return lo.Contains(deletePolicies, common.DeletePolicySucceeded)
}

func (r *AutoDeletableResource) ShouldCleanupWhenFailed() bool {
	deletePolicies := getDeletePolicies(r.unstructured.GetAnnotations())

	return lo.Contains(deletePolicies, common.DeletePolicyFailed)
}
