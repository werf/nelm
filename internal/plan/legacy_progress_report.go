package plan

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/legacy/progrep"
)

func reportOperationStatus(op *Operation, status OperationStatus, reporter *LegacyProgressReporter) {
	op.Status = status

	if reporter == nil {
		return
	}

	reporter.ReportStatus(op.ID(), mapOperationStatus(status))
}

// operationResourceMeta extracts GVK, name, and namespace from an operation's
// config. Returns ok=false for operation configs that don't carry resource metadata
// (e.g. Noop, release operations).
func operationResourceMeta(config OperationConfig) (gvk schema.GroupVersionKind, name, namespace string, ok bool) {
	switch c := config.(type) {
	case *OperationConfigCreate:
		return c.ResourceSpec.GroupVersionKind, c.ResourceSpec.Name, c.ResourceSpec.Namespace, true
	case *OperationConfigUpdate:
		return c.ResourceSpec.GroupVersionKind, c.ResourceSpec.Name, c.ResourceSpec.Namespace, true
	case *OperationConfigApply:
		return c.ResourceSpec.GroupVersionKind, c.ResourceSpec.Name, c.ResourceSpec.Namespace, true
	case *OperationConfigRecreate:
		return c.ResourceSpec.GroupVersionKind, c.ResourceSpec.Name, c.ResourceSpec.Namespace, true
	case *OperationConfigDelete:
		return c.ResourceMeta.GroupVersionKind, c.ResourceMeta.Name, c.ResourceMeta.Namespace, true
	case *OperationConfigTrackReadiness:
		return c.ResourceMeta.GroupVersionKind, c.ResourceMeta.Name, c.ResourceMeta.Namespace, true
	case *OperationConfigTrackPresence:
		return c.ResourceMeta.GroupVersionKind, c.ResourceMeta.Name, c.ResourceMeta.Namespace, true
	case *OperationConfigTrackAbsence:
		return c.ResourceMeta.GroupVersionKind, c.ResourceMeta.Name, c.ResourceMeta.Namespace, true
	default:
		return schema.GroupVersionKind{}, "", "", false
	}
}

func buildResolvedNamespaces(p *Plan, releaseNamespace string, mapper meta.RESTMapper) map[string]string {
	resolved := make(map[string]string)

	for _, op := range p.Operations() {
		if op.Category != OperationCategoryResource && op.Category != OperationCategoryTrack {
			continue
		}

		gvk, _, ns, ok := operationResourceMeta(op.Config)
		if !ok {
			continue
		}

		resolved[op.ID()] = resolveNamespace(gvk, ns, releaseNamespace, mapper)
	}

	return resolved
}

func resolveNamespace(gvk schema.GroupVersionKind, ns, releaseNamespace string, mapper meta.RESTMapper) string {
	namespaced, err := spec.Namespaced(gvk, mapper)
	if err != nil {
		// Graceful CRD fallback: if GVK is unknown, assume namespaced.
		if ns != "" {
			return ns
		}

		return releaseNamespace
	}

	if !namespaced {
		return ""
	}

	if ns != "" {
		return ns
	}

	return releaseNamespace
}

func extractObjectRef(op *Operation, resolvedNamespaces map[string]string) progrep.ObjectRef {
	gvk, name, _, ok := operationResourceMeta(op.Config)
	if !ok {
		panic(fmt.Sprintf("unexpected operation config type %T for operation %s", op.Config, op.ID()))
	}

	return progrep.ObjectRef{
		GroupVersionKind: gvk,
		Name:             name,
		Namespace:        resolvedNamespaces[op.ID()],
	}
}

func mapOperationType(t OperationType) progrep.OperationType {
	switch t {
	case OperationTypeCreate:
		return progrep.OperationTypeCreate
	case OperationTypeUpdate:
		return progrep.OperationTypeUpdate
	case OperationTypeDelete:
		return progrep.OperationTypeDelete
	case OperationTypeApply:
		return progrep.OperationTypeApply
	case OperationTypeRecreate:
		return progrep.OperationTypeRecreate
	case OperationTypeTrackReadiness:
		return progrep.OperationTypeTrackReadiness
	case OperationTypeTrackPresence:
		return progrep.OperationTypeTrackPresence
	case OperationTypeTrackAbsence:
		return progrep.OperationTypeTrackAbsence
	default:
		panic(fmt.Sprintf("unexpected operation type %q", t))
	}
}

func mapOperationStatus(s OperationStatus) progrep.OperationStatus {
	switch s {
	case OperationStatusUnknown:
		return progrep.OperationStatusPending
	case OperationStatusPending:
		return progrep.OperationStatusProgressing
	case OperationStatusCompleted:
		return progrep.OperationStatusCompleted
	case OperationStatusFailed:
		return progrep.OperationStatusFailed
	default:
		return progrep.OperationStatusPending
	}
}
