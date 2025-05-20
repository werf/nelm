package resourceinfo

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/pkg/log"
)

func isImmutableErr(err error) bool {
	return err != nil && errors.IsInvalid(err) && strings.Contains(err.Error(), validation.FieldImmutableErrorMsg)
}

func isNoSuchKindErr(err error) bool {
	return err != nil && meta.IsNoMatchError(err)
}

func isNotFoundErr(err error) bool {
	return err != nil && errors.IsNotFound(err)
}

func fixManagedFieldsInCluster(ctx context.Context, namespace string, getObj *unstructured.Unstructured, getResource *resource.RemoteResource, kubeClient kube.KubeClienter, mapper meta.ResettableRESTMapper) error {
	if changed, err := getResource.FixManagedFields(); err != nil {
		return fmt.Errorf("error fixing managed fields: %w", err)
	} else if !changed {
		return nil
	}

	unstruct := unstructured.Unstructured{Object: map[string]interface{}{}}
	unstruct.SetManagedFields(getResource.Unstructured().GetManagedFields())

	patch, err := json.Marshal(unstruct.UnstructuredContent())
	if err != nil {
		return fmt.Errorf("error marshaling fixed managed fields: %w", err)
	}

	log.Default.Debug(ctx, "Fixing managed fields for resource %q", getResource.HumanID())
	getObj, err = kubeClient.MergePatch(ctx, getResource.ResourceID, patch)
	if err != nil {
		return fmt.Errorf("error patching managed fields: %w", err)
	}

	getResource = resource.NewRemoteResource(getObj, resource.RemoteResourceOptions{
		FallbackNamespace: namespace,
		Mapper:            mapper,
	})

	return nil
}
