package resource

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/samber/lo"
	"github.com/werf/3p-helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
	"sigs.k8s.io/yaml"

	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"

	"github.com/werf/nelm/internal/resource/spec"
)

// Can be called even without cluster access.
func ValidateLocal(ctx context.Context, releaseNamespace string, transformedResources []*InstallableResource,
	opts common.LocalResourceValidationOptions) error {
	if err := validateNoDuplicates(releaseNamespace, transformedResources); err != nil {
		return fmt.Errorf("validate for no duplicated resources: %w", err)
	}

	if !featgate.FeatGateLocalResourceValidation.Enabled() || opts.NoResourceValidation {
		return nil
	}

	if err := validateResourceSchemas(ctx, transformedResources, opts); err != nil {
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
func validateResourceSchemas(ctx context.Context, resources []*InstallableResource, opts common.LocalResourceValidationOptions) error {
	if len(resources) == 0 {
		return nil
	}

	resValidator, err := getSchemaValidator(opts.KubernetesVersion, opts.SkipKinds)
	if err != nil {
		return fmt.Errorf("get schema validator: %w", err)
	}

	var errs []string

	for _, res := range resources {
		if err := validateResourceSchema(ctx, resValidator, res); err != nil {
			errs = append(errs, "\tvalidate "+res.IDHuman()+": "+err.Error())
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("schema validation failed:\n%s", strings.Join(errs, "\n"))
}

// validateResourceSchema validates a single resource using kubeconform
func validateResourceSchema(ctx context.Context, v validator.Validator, res *InstallableResource) error {
	yamlBytes, err := yaml.Marshal(res.Unstruct.Object)
	if err != nil {
		return fmt.Errorf("marshal resource to YAML: %w", err)
	}

	resCh, errCh := resource.FromStream(ctx, res.FilePath, bytes.NewReader(yamlBytes))

	var validationErrs []string

	for validationResource := range resCh {
		validationResult := v.ValidateResource(validationResource)

		if validationResult.Status == validator.Skipped {
			log.Default.Debug(ctx, "Skip local validation for resource: %s", res.IDHuman())
		}

		if validationResult.Status != validator.Valid {
			for _, validationError := range validationResult.ValidationErrors {
				validationErrs = append(validationErrs, validationError.Path+": "+validationError.Msg)
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

func getSchemaValidator(kubeVersion string, skipKinds []string) (validator.Validator, error) {
	schemaLocations := []string{"default"}

	skipKindsMap := lo.SliceToMap(skipKinds, func(s string) (string, struct{}) {
		return s, struct{}{}
	})

	cacheDir, err := getSchemaCacheDir(kubeVersion)
	if err != nil {
		return nil, fmt.Errorf("get schema cache dir: %w", err)
	}

	validatorOpts := validator.Opts{
		Strict:               false, // Skip undefined params check
		IgnoreMissingSchemas: true,
		SkipKinds:            skipKindsMap,
		Cache:                cacheDir,
		KubernetesVersion:    kubeVersion,
	}

	validatorInstance, err := validator.New(schemaLocations, validatorOpts)
	if err != nil {
		return nil, fmt.Errorf("create schema validator: %w", err)
	}

	return validatorInstance, nil
}

func getSchemaCacheDir(kubeVersion string) (string, error) {
	cacheDirPath := helmpath.CachePath(".nelm", "api-resource-json-schemas", kubeVersion)

	if stat, err := os.Stat(cacheDirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDirPath, 0750); err != nil {
			return "", fmt.Errorf("create cache dir: %w", err)
		}

		return cacheDirPath, nil
	} else if err != nil {
		return "", fmt.Errorf("stat cache dir: %w", err)
	} else if !stat.IsDir() {
		return "", fmt.Errorf("%s is not a directory", cacheDirPath)
	}

	return cacheDirPath, nil
}
