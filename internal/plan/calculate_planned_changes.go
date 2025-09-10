package plan

import (
	"github.com/aymanbagabas/go-udiff"

	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

const (
	HiddenCRDOutput            = "<hidden verbose CRD output>"
	HiddenInsignificantOutput  = "<hidden insignificant output>"
	HiddenInsignificantChanges = "<hidden insignificant changes>"
	HiddenSensitiveOutput      = "<hidden sensitive output>"
	HiddenSensitiveChanges     = "<hidden sensitive changes>"
)

// TODO: expose to the user
type CalculatePlannedChangesOptions struct {
	DiffContextLines       int // FIXME(ilya-lesikov): don't forget 3 by default from upper levels
	ShowCRDDiffs           bool
	ShowSensitiveDiffs     bool
	ShowInsignificantDiffs bool
}

func CalculatePlannedChanges(
	installableInfos []*resourceinfo.InstallableResourceInfo,
	deletableInfos []*resourceinfo.DeletableResourceInfo,
	opts CalculatePlannedChangesOptions,
) (
	createdChanges []*CreatedResourceChange,
	recreatedChanges []*RecreatedResourceChange,
	updatedChanges []*UpdatedResourceChange,
	appliedChanges []*AppliedResourceChange,
	deletedChanges []*DeletedResourceChange,
	anyChangesPlanned bool,
) {
	var instInfosByIter [][]*resourceinfo.InstallableResourceInfo
	for _, instInfo := range installableInfos {
		if len(instInfosByIter) < instInfo.Iteration + 1 {
			instInfosByIter = append(instInfosByIter, []*resourceinfo.InstallableResourceInfo{})
		}

		instInfosByIter[instInfo.Iteration] = append(instInfosByIter[instInfo.Iteration], instInfo)
	}

	for iter, instInfos := range instInfosByIter {
		if iter == 0 {
			for _, instInfo := range instInfos {
				// FIXME(ilya-lesikov): move to a separate function?
				var uDiff string
				if resource.IsCRD(instInfo.GroupVersionKind.GroupKind()) {
					uDiff = HiddenCRDOutput
				} else if sensitiveInfo := resource.GetSensitiveInfo(instInfo.GroupVersionKind.GroupKind(), instInfo.Annotations); sensitiveInfo.IsSensitive {
					if sensitiveInfo.FullySensitive()
				}
			}
		}
	}

	for _, op := range plan.Operations() {
		switch op.Type {
		case operation.OperationTypeCreate:
			opConfig := op.Config.(*operation.OperationConfigCreate)

			var uDiff string
			if resource.IsCRD(opConfig.ResourceSpec.GroupVersionKind.GroupKind()) {
				uDiff = HiddenCRDOutput
			} else if sensitiveInfo := resource.GetSensitiveInfo(opConfig.ResourceSpec.GroupVersionKind.GroupKind(), opConfig.ResourceSpec.Annotations); sensitiveInfo.IsSensitive {
				if sensitiveInfo.FullySensitive() {
					uDiff = HiddenSensitiveOutput
				} else {
					cleanedUp
				}
			}

			isCRD :=

		case operation.OperationTypeRecreate:
			opConfig := op.Config.(*operation.OperationConfigRecreate)
			sensitiveInfo := resource.GetSensitiveInfo(opConfig.ResourceSpec.GroupVersionKind.GroupKind(), opConfig.ResourceSpec.Annotations)
		case operation.OperationTypeUpdate:
			opConfig := op.Config.(*operation.OperationConfigUpdate)
			sensitiveInfo := resource.GetSensitiveInfo(opConfig.ResourceSpec.GroupVersionKind.GroupKind(), opConfig.ResourceSpec.Annotations)
		case operation.OperationTypeApply:
			opConfig := op.Config.(*operation.OperationConfigApply)
			sensitiveInfo := resource.GetSensitiveInfo(opConfig.ResourceSpec.GroupVersionKind.GroupKind(), opConfig.ResourceSpec.Annotations)
		case operation.OperationTypeDelete:
			opConfig := op.Config.(*operation.OperationConfigDelete)
			sensitiveInfo := resource.GetSensitiveInfo(opConfig.ResourceSpec.GroupVersionKind.GroupKind(), opConfig.ResourceSpec.Annotations)
		}
	}
}

type ResourceChange struct {
	*id.ResourceMeta

	Udiff           string
	ExtraOperations []string
}
