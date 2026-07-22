//go:build ai_tests

package kube

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestAI_IsInvalidErr(t *testing.T) {
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

func TestAI_IsTypedObjectErr(t *testing.T) {
	typedObjErr := fmt.Errorf(`server-side dry-run apply resource "DaemonSet/log-shipper-agent": server-side apply: failed to create typed patch object (d8-log-shipper/log-shipper-agent; apps/v1, Kind=DaemonSet): .spec.template.spec.containers[name="vector"].resources.cpu: field not declared in schema`)

	assert.True(t, IsTypedObjectErr(typedObjErr))
	assert.True(t, IsTypedObjectErr(fmt.Errorf("wrapped: %w", typedObjErr)))

	assert.False(t, IsTypedObjectErr(nil))
	assert.False(t, IsTypedObjectErr(apierrors.NewServiceUnavailable("try later")))
	assert.False(t, IsTypedObjectErr(apierrors.NewInternalError(errors.New("boom"))))
	assert.False(t, IsTypedObjectErr(errors.New("failed to create typed live object")))
}

func TestAI_TypedObjectErrIsNotStatusInvalid(t *testing.T) {
	rawTypedObjErr := fmt.Errorf("failed to create typed patch object: field not declared in schema")
	serverErr := apierrors.NewGenericServerResponse(500, "PATCH", schema.GroupResource{}, "", rawTypedObjErr.Error(), 0, false)

	assert.False(t, IsInvalidErr(serverErr))
	assert.True(t, IsTypedObjectErr(serverErr))
}
