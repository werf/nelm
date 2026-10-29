package plan

import (
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

// Whether executing the plan justifies a new release revision. Operations on resources that don't
// trigger releases are not counted. If there are such operations and no resource operations
// besides them, then tracking of other resources is not counted either: such tracking only waits
// for unchanged resources, e.g. dependencies of the non-triggering resources.
func PlanTriggersRelease(p *Plan) bool {
	var hasNonTriggeringOps, hasTrackOps bool
	for _, op := range p.Operations() {
		if op.Category != OperationCategoryResource && op.Category != OperationCategoryTrack {
			continue
		}

		if !operationTriggersRelease(op) {
			hasNonTriggeringOps = true

			continue
		}

		if op.Category == OperationCategoryResource {
			return true
		}

		hasTrackOps = true
	}

	return hasTrackOps && !hasNonTriggeringOps
}

func operationTriggersRelease(op *Operation) bool {
	meta, ok := operationConfigResourceMeta(op.Config)

	return !ok || resource.TriggersRelease(meta.Annotations)
}

func operationConfigResourceMeta(config OperationConfig) (*spec.ResourceMeta, bool) {
	switch c := config.(type) {
	case *OperationConfigCreate:
		return c.ResourceSpec.ResourceMeta, true
	case *OperationConfigRecreate:
		return c.ResourceSpec.ResourceMeta, true
	case *OperationConfigUpdate:
		return c.ResourceSpec.ResourceMeta, true
	case *OperationConfigApply:
		return c.ResourceSpec.ResourceMeta, true
	case *OperationConfigDelete:
		return c.ResourceMeta, true
	case *OperationConfigTrackReadiness:
		return c.ResourceMeta, true
	case *OperationConfigTrackPresence:
		return c.ResourceMeta, true
	case *OperationConfigTrackAbsence:
		return c.ResourceMeta, true
	default:
		return nil, false
	}
}
