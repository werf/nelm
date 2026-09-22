package kube

import (
	"regexp"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

func IsImmutableErr(err error) bool {
	if err == nil || !errors.IsInvalid(err) {
		return false
	}

	// Kinds with custom update validation (StatefulSet, PersistentVolumeClaim, Pod, Service, StorageClass, ...)
	// report immutable field changes with their own wording instead of validation.FieldImmutableErrorMsg.
	immutableErrRegexps := []*regexp.Regexp{
		regexp.MustCompile(`\bfield is immutable\b`),
		regexp.MustCompile(`\bis immutable after creation\b`),
		regexp.MustCompile(`\bupdates to \S+ are forbidden\b`),
		regexp.MustCompile(`\bupdates to statefulset spec for fields other than .* are forbidden\b`),
		regexp.MustCompile(`\bpod updates may not change fields other than\b`),
		regexp.MustCompile(`\bresources for non-sidecar init containers are immutable\b`),
		regexp.MustCompile(`\bmay not change once set\b`),
	}

	return lo.SomeBy(immutableErrRegexps, func(re *regexp.Regexp) bool {
		return re.MatchString(err.Error())
	})
}

func IsInvalidErr(err error) bool {
	return err != nil && errors.IsInvalid(err)
}

func IsNoSuchKindErr(err error) bool {
	return err != nil && meta.IsNoMatchError(err)
}

func IsNotFoundErr(err error) bool {
	return err != nil && errors.IsNotFound(err)
}

func IsTypedObjectErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "failed to create typed patch object")
}

func IsWebhookErr(err error) bool {
	return err != nil &&
		(strings.Contains(err.Error(), "failed calling webhook") ||
			strings.Contains(err.Error(), "conversion webhook for"))
}
