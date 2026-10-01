package kube

import (
	"errors"
	"regexp"
	"strings"

	"github.com/samber/lo"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var immutableErrRegexps = []*regexp.Regexp{
	// apimachinery ValidateImmutableField; Secret/ConfigMap with immutable: true.
	regexp.MustCompile(`\b` + regexp.QuoteMeta(validation.FieldImmutableErrorMsg) + `\b`),
	// PersistentVolume, PersistentVolumeClaim, Job schedulingPolicy.
	regexp.MustCompile(`\bis immutable after creation\b`),
	// StatefulSet.
	regexp.MustCompile(`: updates to statefulset spec for fields other than .* are forbidden\b`),
	// StorageClass, VolumeAttributesClass.
	regexp.MustCompile(`: updates to (parameters|provisioner|reclaimPolicy|driverName) are forbidden\b`),
	// Pod.
	regexp.MustCompile(`: pod updates may not (change fields other than|add or remove containers)\b`),
	regexp.MustCompile(`: existing toleration can not be modified except its tolerationSeconds\b`),
	// Service clusterIPs, ipFamilies, loadBalancerClass.
	regexp.MustCompile(`: may not change once set$`),
	// RoleBinding, ClusterRoleBinding.
	regexp.MustCompile(`: cannot change roleRef\b`),
	// PriorityClass.
	regexp.MustCompile(`: may not be changed in an update\.$`),
}

func IsImmutableErr(err error) bool {
	if err == nil || !apierrors.IsInvalid(err) {
		return false
	}

	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) {
		return false
	}

	// Status.Message of an API server response is the causes joined into one string, so it is
	// only consulted when there are no causes (a validating webhook): otherwise the ".*" of one
	// pattern could span two unrelated causes.
	messages := []string{statusErr.ErrStatus.Message}
	if statusErr.ErrStatus.Details != nil && len(statusErr.ErrStatus.Details.Causes) > 0 {
		messages = lo.Map(statusErr.ErrStatus.Details.Causes, func(cause metav1.StatusCause, _ int) string {
			return cause.Message
		})
	}

	return lo.SomeBy(messages, func(message string) bool {
		return lo.SomeBy(immutableErrRegexps, func(re *regexp.Regexp) bool {
			return re.MatchString(message)
		})
	})
}

func IsInvalidErr(err error) bool {
	return err != nil && apierrors.IsInvalid(err)
}

func IsNoSuchKindErr(err error) bool {
	return err != nil && meta.IsNoMatchError(err)
}

func IsNotFoundErr(err error) bool {
	return err != nil && apierrors.IsNotFound(err)
}

func IsTypedObjectErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "failed to create typed patch object")
}

func IsWebhookErr(err error) bool {
	return err != nil &&
		(strings.Contains(err.Error(), "failed calling webhook") ||
			strings.Contains(err.Error(), "conversion webhook for"))
}
