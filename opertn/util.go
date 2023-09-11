package opertn

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/kubeclnt"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
)

// TODO(ilya-lesikov): can we do this in a single apply request?
func doRepairManagedFields(ctx context.Context, resource *resrcid.ResourceID, kubeClient kubeclnt.KubeClienter) error {
	getObj, err := kubeClient.Get(ctx, resource)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("error getting resource: %w", err)
	}

	liveManagedFields := getObj.GetManagedFields()
	if len(liveManagedFields) == 0 {
		return nil
	}

	var updatedManagedFields []v1.ManagedFieldsEntry
	for _, entry := range liveManagedFields {
		if entry.Manager == common.OldFieldManager {
			entry.Manager = common.DefaultFieldManager
		} else if entry.Manager == common.KubectlEditFieldManager {
			continue
		}
		updatedManagedFields = append(updatedManagedFields, entry)
	}

	unstruct := unstructured.Unstructured{Object: map[string]interface{}{}}
	unstruct.SetManagedFields(updatedManagedFields)

	patch, err := json.Marshal(unstruct)
	if err != nil {
		return fmt.Errorf("error marshaling updated managed fields for resource %q: %w", resource.HumanID(), err)
	}

	if _, err := kubeClient.StrategicPatch(ctx, resource, patch); err != nil {
		return fmt.Errorf("error patching resource %q with updated managed fields: %w", resource.HumanID(), err)
	}

	return nil
}
