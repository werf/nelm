package validate

// import (
// 	"context"
// 	"fmt"
//
// 	"helm.sh/helm/v3/pkg/werf/common"
// 	"helm.sh/helm/v3/pkg/werf/resource/kuberesource"
// )
//
// func NewAdoptionValidator(releaseName, releaseNamespace string) LocalValidator {
// 	return &AdoptionValidator{
// 		releaseName:      releaseName,
// 		releaseNamespace: releaseNamespace,
// 	}
// }
//
// type AdoptionValidator struct {
// 	releaseName      string
// 	releaseNamespace string
// }
//
// func (v *AdoptionValidator) LocalValidate(ctx context.Context, opts LocalValidateOptions, resources ...kuberesource.LocalKubeResourcer) ([]ValidationError, error) {
// 	for _, res := range resources {
// 		if opts.SelectorFn != nil {
// 			if shouldSelect, err := opts.SelectorFn(res); err != nil {
// 				return nil, fmt.Errorf("failed to select resource: %w", err)
// 			} else if !shouldSelect {
// 				continue
// 			}
// 		}
//
// 		res.Unstructured().GetAnnotations()[string(common.AnnotationReleaseName)]
// 	}
//
// 	var errors []ValidationError
//
// 	for _, resource := range selectedResources {
// 		if err := v.validateResource(resource); err != nil {
// 			errors = append(errors, err)
// 		}
// 	}
//
// 	return errors, nil
// }
