package resource

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"

	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"

	"github.com/werf/nelm/internal/resource/spec"
)

// Can be called even without cluster access.
func ValidateLocal(releaseNamespace string, transformedResources []*InstallableResource) error {
	if err := validateNoDuplicates(releaseNamespace, transformedResources); err != nil {
		return fmt.Errorf("validate for no duplicated resources: %w", err)
	}

	if err := validateResourceSchemas(transformedResources); err != nil {
		return fmt.Errorf("validate resource schemas: %w", err)
	}

	return nil
}

func validateNoDuplicates(releaseNamespace string, transformedResources []*InstallableResource) error {
	for _, res := range transformedResources {
		if spec.IsReleaseNamespace(res.Unstruct.GetName(), res.Unstruct.GroupVersionKind(), releaseNamespace) {
			return fmt.Errorf("release namespace %q cannot be deployed as part of the release", res.Unstruct.GetName())
		}
	}

	duplicates := lo.FindDuplicatesBy(transformedResources, func(instRes *InstallableResource) string {
		return instRes.ID()
	})

	duplicates = lo.Filter(duplicates, func(instRes *InstallableResource, _ int) bool {
		return !spec.IsWebhook(instRes.GroupVersionKind.GroupKind())
	})

	if len(duplicates) == 0 {
		return nil
	}

	duplicatedIDHumans := lo.Map(duplicates, func(instRes *InstallableResource, _ int) string {
		return instRes.IDHuman()
	})

	return fmt.Errorf("duplicated resources found: %s", strings.Join(duplicatedIDHumans, ", "))
}

// validateResourceSchemas validates that resources conform to their Kubernetes API schemas
// by attempting to decode them into typed objects using scheme.Codecs.
func validateResourceSchemas(transformedResources []*InstallableResource) error {
	// Filter out resources that should be skipped
	resourcesToValidate := lo.Filter(transformedResources, func(res *InstallableResource, _ int) bool {
		return !shouldSkipSchemaValidation(res)
	})

	if len(resourcesToValidate) == 0 {
		return nil
	}

	schemaLocations := []string{"default"}
	opts := validator.Opts{
		Strict:               true, // Strict undefined params check
		IgnoreMissingSchemas: true,
		KubernetesVersion:    "", // Use latest available
	}

	validatorInstance, err := validator.New(schemaLocations, opts)
	if err != nil {
		return fmt.Errorf("create schema validator: %w", err)
	}

	// TODO: need to pass real context
	ctx := context.TODO()
	var errs []string

	for _, res := range resourcesToValidate {
		if err := validateResourceWithKubeconform(ctx, validatorInstance, res); err != nil {
			errs = append(errs, "\tvalidate "+res.IDHuman()+": "+err.Error())
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("schema validation failed:\n%s", strings.Join(errs, "\n"))
}

// validateResourceWithKubeconform validates a single resource using kubeconform
func validateResourceWithKubeconform(ctx context.Context, v validator.Validator, res *InstallableResource) error {
	yamlBytes, err := yaml.Marshal(res.Unstruct.Object)
	if err != nil {
		return fmt.Errorf("marshal resource to YAML: %w", err)
	}

	resCh, errCh := resource.FromStream(ctx, res.FilePath, bytes.NewReader(yamlBytes))

	var validationErrs []string

	for validationResource := range resCh {
		validationResult := v.ValidateResource(validationResource)

		if validationResult.Status != validator.Valid {
			for _, validationError := range validationResult.ValidationErrors {
				validationErrs = append(validationErrs, validationError.Msg)
			}
		}
	}

	// Check for stream reading errors
	for err := range errCh {
		if err != nil {
			return fmt.Errorf("read resource stream: %w", err)
		}
	}

	if len(validationErrs) > 0 {
		return fmt.Errorf("%s", strings.Join(validationErrs, "; "))
	}

	return nil
}

// shouldSkipSchemaValidation returns true if the resource should be skipped from schema validation.
// This includes CRD definitions and Custom Resources (instances of CRDs).
func shouldSkipSchemaValidation(res *InstallableResource) bool {
	gvk := res.Unstruct.GroupVersionKind()

	if spec.IsCRD(gvk.GroupKind()) {
		return true
	}

	if _, err := scheme.Scheme.New(gvk); err != nil {
		return true
	}

	return false
}
