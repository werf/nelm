package resourceinfo

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

func fixManagedFieldsInCluster(ctx context.Context, releaseNamespace string, getObj *unstructured.Unstructured, meta *id.ResourceMeta, kubeClient kube.KubeClienter) (*unstructured.Unstructured, error) {
	if changed, err := fixManagedFields(getObj); err != nil {
		return nil, fmt.Errorf("fix managed fields for resource %q: %w", meta.IDHuman(), err)
	} else if !changed {
		return getObj, nil
	}

	unstruct := unstructured.Unstructured{Object: map[string]interface{}{}}
	unstruct.SetManagedFields(unstruct.GetManagedFields())

	patch, err := json.Marshal(unstruct.UnstructuredContent())
	if err != nil {
		return nil, fmt.Errorf("marshal fixed managed fields for resource %q: %w", meta.IDHuman(), err)
	}

	log.Default.Debug(ctx, "Fixing managed fields for resource %q", meta.IDHuman())
	getObj, err = kubeClient.MergePatch(ctx, meta, patch)
	if err != nil {
		return nil, fmt.Errorf("patch managed fields: %w", err)
	}

	return getObj, nil
}

func fixManagedFields(unstruct *unstructured.Unstructured) (changed bool, err error) {
	managedFields := unstruct.GetManagedFields()
	if len(managedFields) == 0 {
		return false, nil
	}

	var oursEntry v1.ManagedFieldsEntry
	if e, found := lo.Find(managedFields, func(e v1.ManagedFieldsEntry) bool {
		return e.Manager == common.DefaultFieldManager && e.Operation == v1.ManagedFieldsOperationApply
	}); found {
		oursEntry = e
	} else {
		oursEntry = v1.ManagedFieldsEntry{
			Manager:    common.DefaultFieldManager,
			Operation:  v1.ManagedFieldsOperationApply,
			APIVersion: unstruct.GetAPIVersion(),
			Time:       lo.ToPtr(v1.Now()),
			FieldsType: "FieldsV1",
			FieldsV1:   &v1.FieldsV1{Raw: []byte("{}")},
		}
	}

	var fixedManagedFields []v1.ManagedFieldsEntry

	fixedManagedFields = append(fixedManagedFields, differentSubresourceManagers(managedFields, oursEntry)...)

	if newManagedFields, newOursEntry, chngd := removeUndesirableManagers(managedFields, oursEntry); chngd {
		fixedManagedFields = append(fixedManagedFields, newManagedFields...)
		oursEntry = newOursEntry
		changed = true
	}

	if newManagedFields, chngd := exclusiveOwnershipForOurManager(managedFields, oursEntry); chngd {
		fixedManagedFields = append(fixedManagedFields, newManagedFields...)
		changed = true
	}

	if string(oursEntry.FieldsV1.Raw) != "{}" {
		fixedManagedFields = append(fixedManagedFields, oursEntry)
	}

	if changed {
		unstruct.SetManagedFields(fixedManagedFields)
	}

	return changed, nil
}

func differentSubresourceManagers(managedFields []v1.ManagedFieldsEntry, oursEntry v1.ManagedFieldsEntry) (newManagedFields []v1.ManagedFieldsEntry) {
	for _, managedField := range managedFields {
		if managedField.Subresource != oursEntry.Subresource {
			newManagedFields = append(newManagedFields, managedField)
			continue
		}
	}

	return newManagedFields
}

func removeUndesirableManagers(managedFields []v1.ManagedFieldsEntry, oursEntry v1.ManagedFieldsEntry) (newManagedFields []v1.ManagedFieldsEntry, newOursEntry v1.ManagedFieldsEntry, changed bool) {
	oursFieldsByte := lo.Must(json.Marshal(oursEntry.FieldsV1))

	newOursEntry = oursEntry
	for _, managedField := range managedFields {
		if managedField.Subresource != oursEntry.Subresource {
			continue
		}

		fieldsByte := lo.Must(json.Marshal(managedField.FieldsV1))

		if managedField.Manager == common.DefaultFieldManager {
			if managedField.Operation == v1.ManagedFieldsOperationApply {
				continue
			}

			merged, mergeChanged := lo.Must2(util.MergeJson(fieldsByte, oursFieldsByte))
			if mergeChanged {
				oursFieldsByte = merged
				lo.Must0(newOursEntry.FieldsV1.UnmarshalJSON(merged))
			}

			changed = true
		} else if managedField.Manager == common.KubectlEditFieldManager ||
			strings.HasPrefix(managedField.Manager, common.OldFieldManagerPrefix) {
			merged, mergeChanged := lo.Must2(util.MergeJson(fieldsByte, oursFieldsByte))
			if mergeChanged {
				oursFieldsByte = merged
				lo.Must0(newOursEntry.FieldsV1.UnmarshalJSON(merged))
			}

			changed = true
		}
	}

	return newManagedFields, newOursEntry, changed
}

func exclusiveOwnershipForOurManager(managedFields []v1.ManagedFieldsEntry, oursEntry v1.ManagedFieldsEntry) (newManagedFields []v1.ManagedFieldsEntry, changed bool) {
	oursFieldsByte := lo.Must(json.Marshal(oursEntry.FieldsV1))

	for _, managedField := range managedFields {
		if managedField.Subresource != oursEntry.Subresource {
			continue
		}

		fieldsByte := lo.Must(json.Marshal(managedField.FieldsV1))

		if managedField.Manager == common.DefaultFieldManager ||
			managedField.Manager == common.KubectlEditFieldManager ||
			strings.HasPrefix(managedField.Manager, common.OldFieldManagerPrefix) {
			continue
		}

		subtracted, subtractChanged := lo.Must2(util.SubtractJson(fieldsByte, oursFieldsByte))
		if !subtractChanged {
			newManagedFields = append(newManagedFields, managedField)
			continue
		}

		if string(subtracted) != "{}" {
			lo.Must0(managedField.FieldsV1.UnmarshalJSON(subtracted))
			newManagedFields = append(newManagedFields, managedField)
		}

		changed = true
	}

	return newManagedFields, changed
}
