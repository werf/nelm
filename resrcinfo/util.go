package resrcinfo

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/validation"
)

func isImmutableErr(err error) bool {
	return err != nil && errors.IsInvalid(err) && strings.Contains(err.Error(), validation.FieldImmutableErrorMsg)
}

func isNoSuchKindErr(err error) bool {
	return err != nil && meta.IsNoMatchError(err)
}

func isNotFoundErr(err error) bool {
	return err != nil && errors.IsNotFound(err)
}
