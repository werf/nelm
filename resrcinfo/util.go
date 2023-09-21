package resrcinfo

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
)

func isImmutableErr(err error) bool {
	return err != nil && errors.IsInvalid(err) && strings.Contains(err.Error(), "field is immutable")
}

func isNoSuchKindErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no matches for kind")
}

func isNotFoundErr(err error) bool {
	return err != nil && errors.IsNotFound(err)
}
