package kube

import (
	"regexp"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

var immutableErrRegexps = []*regexp.Regexp{
	regexp.MustCompile(`\bfield is immutable\b`),
	regexp.MustCompile(`\bis immutable after creation\b`),
	regexp.MustCompile(`\bupdates to .+ are forbidden\b`),
	regexp.MustCompile(`\bpod updates may not change fields other than\b`),
	regexp.MustCompile(`\bmay not change once set\b`),
	regexp.MustCompile(`\bcannot change roleRef\b`),
	regexp.MustCompile(`\bmay not be changed in an update\b`),
}

func IsImmutableErr(err error) bool {
	if err == nil || !errors.IsInvalid(err) {
		return false
	}

	msg := err.Error()

	return lo.SomeBy(immutableErrRegexps, func(re *regexp.Regexp) bool {
		return re.MatchString(msg)
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
