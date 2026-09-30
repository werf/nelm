package kube

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestIsImmutableErr(t *testing.T) {
	statefulSetGK := schema.GroupKind{Group: "apps", Kind: "StatefulSet"}
	pvcGK := schema.GroupKind{Kind: "PersistentVolumeClaim"}

	apimachineryImmutableErr := apierrors.NewInvalid(schema.GroupKind{Group: "batch", Kind: "Job"}, "backup", field.ErrorList{
		field.Invalid(field.NewPath("spec", "selector"), "", "field is immutable"),
	})
	statefulSetErr := apierrors.NewInvalid(statefulSetGK, "db", field.ErrorList{
		field.Forbidden(field.NewPath("spec"), "updates to statefulset spec for fields other than 'replicas', 'ordinals', 'template', 'updateStrategy', 'revisionHistoryLimit', 'persistentVolumeClaimRetentionPolicy' and 'minReadySeconds' are forbidden"),
	})
	storageClassErr := apierrors.NewInvalid(schema.GroupKind{Group: "storage.k8s.io", Kind: "StorageClass"}, "fast", field.ErrorList{
		field.Forbidden(field.NewPath("provisioner"), "updates to provisioner are forbidden."),
	})
	pvcErr := apierrors.NewInvalid(pvcGK, "data", field.ErrorList{
		field.Forbidden(field.NewPath("spec"), "spec is immutable after creation except resources.requests and volumeAttributesClassName for bound claims\n  core.PersistentVolumeClaimSpec{...}"),
	})
	podErr := apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "debug", field.ErrorList{
		field.Forbidden(field.NewPath("spec"), "pod updates may not change fields other than `spec.containers[*].image`,`spec.initContainers[*].image`,`spec.activeDeadlineSeconds`,`spec.tolerations` (only additions to existing tolerations),`spec.terminationGracePeriodSeconds` (allow it to be set to 1 if it was previously negative)\n  core.PodSpec{...}"),
	})
	serviceErr := apierrors.NewInvalid(schema.GroupKind{Kind: "Service"}, "web", field.ErrorList{
		field.Invalid(field.NewPath("spec", "clusterIPs").Index(0), []string{"10.0.0.1"}, "may not change once set"),
	})
	roleBindingErr := apierrors.NewInvalid(schema.GroupKind{Group: "rbac.authorization.k8s.io", Kind: "RoleBinding"}, "app", field.ErrorList{
		field.Invalid(field.NewPath("roleRef"), "", "cannot change roleRef"),
	})
	priorityClassErr := apierrors.NewInvalid(schema.GroupKind{Group: "scheduling.k8s.io", Kind: "PriorityClass"}, "high", field.ErrorList{
		field.Forbidden(field.NewPath("value"), "may not be changed in an update."),
	})
	podContainersErr := apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "debug", field.ErrorList{
		field.Forbidden(field.NewPath("spec", "containers"), "pod updates may not add or remove containers"),
	})
	podTolerationErr := apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "debug", field.ErrorList{
		field.Forbidden(field.NewPath("spec", "tolerations").Index(0), "existing toleration can not be modified except its tolerationSeconds"),
	})

	assert.True(t, IsImmutableErr(apimachineryImmutableErr))
	assert.True(t, IsImmutableErr(statefulSetErr))
	assert.True(t, IsImmutableErr(storageClassErr))
	assert.True(t, IsImmutableErr(pvcErr))
	assert.True(t, IsImmutableErr(podErr))
	assert.True(t, IsImmutableErr(serviceErr))
	assert.True(t, IsImmutableErr(roleBindingErr))
	assert.True(t, IsImmutableErr(priorityClassErr))
	assert.True(t, IsImmutableErr(podContainersErr))
	assert.True(t, IsImmutableErr(podTolerationErr))
	assert.True(t, IsImmutableErr(fmt.Errorf("retryable on webhook error: %w", statefulSetErr)))
	assert.True(t, IsImmutableErr(&apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusUnprocessableEntity,
		Reason:  metav1.StatusReasonInvalid,
		Message: `admission webhook "validate.example.com" denied the request: spec.storage: field is immutable`,
		Details: &metav1.StatusDetails{Group: "example.com", Kind: "Widget", Name: "w"},
	}}))

	assert.False(t, IsImmutableErr(nil))
	assert.False(t, IsImmutableErr(apierrors.NewInvalid(statefulSetGK, "db", field.ErrorList{
		field.Invalid(field.NewPath("spec", "replicas"), -1, "must be greater than or equal to 0"),
	})))
	assert.False(t, IsImmutableErr(apierrors.NewInvalid(statefulSetGK, "db", field.ErrorList{
		field.Required(field.NewPath("spec", "selector"), ""),
	})))
	assert.False(t, IsImmutableErr(apierrors.NewInvalid(pvcGK, "data", field.ErrorList{
		field.Forbidden(field.NewPath("spec", "resources", "requests", "storage"), "field can not be less than previous value"),
	})))
	assert.False(t, IsImmutableErr(apierrors.NewInvalid(schema.GroupKind{Group: "example.com", Kind: "Widget"}, "w", field.ErrorList{
		field.Invalid(field.NewPath("spec", "count"), -1, "negative values are forbidden"),
	})))
	assert.False(t, IsImmutableErr(apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "statefulsets"}, "db", errors.New("updates to statefulset spec are forbidden"))))
	assert.False(t, IsImmutableErr(errors.New("updates to statefulset spec are forbidden")))
	assert.False(t, IsImmutableErr(apierrors.NewInvalid(schema.GroupKind{Group: "example.com", Kind: "Widget"}, "w", field.ErrorList{
		field.Forbidden(field.NewPath("spec", "size"), "updates to size"),
		field.Invalid(field.NewPath("spec", "count"), -1, "negative values are forbidden"),
	})))
	assert.False(t, IsImmutableErr(apierrors.NewInvalid(schema.GroupKind{Group: "example.com", Kind: "Widget"}, "w", field.ErrorList{
		field.Forbidden(field.NewPath("spec", "replicas"), "updates to replicas are forbidden while the rollout is paused"),
	})))
}

func TestIsInvalidErr(t *testing.T) {
	invalidErr := apierrors.NewInvalid(schema.GroupKind{}, "", field.ErrorList{
		field.Invalid(field.NewPath("patch"), "", "bad"),
	})

	assert.True(t, IsInvalidErr(invalidErr))
	assert.True(t, IsInvalidErr(fmt.Errorf("wrapped: %w", invalidErr)))

	assert.False(t, IsInvalidErr(nil))
	assert.False(t, IsInvalidErr(apierrors.NewServiceUnavailable("try later")))
	assert.False(t, IsInvalidErr(apierrors.NewTimeoutError("timed out", 1)))
	assert.False(t, IsInvalidErr(apierrors.NewInternalError(errors.New("boom"))))
	assert.False(t, IsInvalidErr(errors.New("connection refused")))
}

func TestIsTypedObjectErr(t *testing.T) {
	typedObjErr := fmt.Errorf(`server-side dry-run apply resource "DaemonSet/log-shipper-agent": server-side apply: failed to create typed patch object (d8-log-shipper/log-shipper-agent; apps/v1, Kind=DaemonSet): .spec.template.spec.containers[name="vector"].resources.cpu: field not declared in schema`)

	assert.True(t, IsTypedObjectErr(typedObjErr))
	assert.True(t, IsTypedObjectErr(fmt.Errorf("wrapped: %w", typedObjErr)))

	assert.False(t, IsTypedObjectErr(nil))
	assert.False(t, IsTypedObjectErr(apierrors.NewServiceUnavailable("try later")))
	assert.False(t, IsTypedObjectErr(apierrors.NewInternalError(errors.New("boom"))))
	assert.False(t, IsTypedObjectErr(errors.New("failed to create typed live object")))
}

func TestTypedObjectErrIsNotStatusInvalid(t *testing.T) {
	rawTypedObjErr := fmt.Errorf("failed to create typed patch object: field not declared in schema")
	serverErr := apierrors.NewGenericServerResponse(500, "PATCH", schema.GroupResource{}, "", rawTypedObjErr.Error(), 0, false)

	assert.False(t, IsInvalidErr(serverErr))
	assert.True(t, IsTypedObjectErr(serverErr))
}
