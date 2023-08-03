package resourcev2

import (
	"regexp"

	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/werf/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyHumanDeletePolicy = "werf.io/delete-policy"
var annotationKeyPatternDeletePolicy = regexp.MustCompile(`^werf.io/delete-policy$`)

var annotationKeyHumanHookDeletePolicy = "helm.sh/hook-delete-policy"
var annotationKeyPatternHookDeletePolicy = regexp.MustCompile(`^helm.sh/hook-delete-policy$`)

func newRecreatableResource(unstruct *unstructured.Unstructured) *recreatableResource {
	return &recreatableResource{unstructured: unstruct}
}

type recreatableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *recreatableResource) Validate() error {
	if err := validateDeletePolicyAnnotations(r.unstructured.GetAnnotations()); err != nil {
		return err
	}

	return nil
}

func (r *recreatableResource) ShouldRecreate() bool {
	deletePolicies := getDeletePolicies(r.unstructured.GetAnnotations())

	return lo.Contains(deletePolicies, common.DeletePolicyBeforeCreation)
}
