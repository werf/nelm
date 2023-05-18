package validate

// import (
// 	"context"
// 	"fmt"
//
// 	"helm.sh/helm/v3/pkg/werf/client"
// 	"helm.sh/helm/v3/pkg/werf/resource/kuberesource"
// )

// func NewDryRunValidator(client *client.Client) RemoteValidator {
// 	return &DryRunValidator{
// 		client: client,
// 	}
// }
//
// type DryRunValidator struct {
// 	client *client.Client
// }
//
// func (v *DryRunValidator) RemoteValidate(ctx context.Context, opts RemoteValidateOptions, resources ...kuberesource.RemoteKubeResourcer) ([]ValidationError, error) {
// 	var selectedResources []kuberesource.RemoteKubeResourcer
// 	if opts.SelectorFn != nil {
// 		for _, resource := range resources {
// 			if shouldSelect, err := opts.SelectorFn(resource); err != nil {
// 				return nil, fmt.Errorf("failed to select resource: %w", err)
// 			} else if shouldSelect {
// 				selectedResources = append(selectedResources, resource)
// 			}
// 		}
// 	} else {
// 		selectedResources = resources
// 	}
// 	if selectedResources == nil {
// 		return nil, nil
// 	}
//
// 	if _, err := v.client.SmartApply(ctx, client.SmartApplyOptions{DryRun: true}, selectedResources...); err != nil {
// 		return []ValidationError{NewValidationError("dry-run apply failed: %s", err)}, nil
// 	}
//
// 	return nil, nil
// }
