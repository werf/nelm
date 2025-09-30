package kube

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/validation"
)

func IsImmutableErr(err error) bool {
	return err != nil && errors.IsInvalid(err) && strings.Contains(err.Error(), validation.FieldImmutableErrorMsg)
}

func IsNoSuchKindErr(err error) bool {
	return err != nil && meta.IsNoMatchError(err)
}

func IsNotFoundErr(err error) bool {
	return err != nil && errors.IsNotFound(err)
}
