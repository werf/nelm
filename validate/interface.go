package validate

// import (
// 	"context"
// 	"fmt"
//
// 	"helm.sh/helm/v3/pkg/werf/resource/kuberesource"
// )
//
// type LocalValidator interface {
// 	LocalValidate(ctx context.Context, options LocalValidateOptions, resources ...kuberesource.LocalKubeResourcer) ([]ValidationError, error)
// }
//
// type RemoteValidator interface {
// 	RemoteValidate(ctx context.Context, options RemoteValidateOptions, resources ...kuberesource.RemoteKubeResourcer) ([]ValidationError, error)
// }
//
// type LocalValidateOptions struct {
// 	SelectorFn func(res kuberesource.LocalKubeResourcer) (bool, error)
// }
//
// type RemoteValidateOptions struct {
// 	SelectorFn func(res kuberesource.RemoteKubeResourcer) (bool, error)
// }
//
// func NewValidationError(format string, a ...any) ValidationError {
// 	return ValidationError{
// 		format: format,
// 		args:   a,
// 	}
// }
//
// type ValidationError struct {
// 	format string
// 	args   []any
// }
//
// func (e *ValidationError) Error() string {
// 	return "validation error: " + fmt.Sprintf(e.format, e.args...)
// }
