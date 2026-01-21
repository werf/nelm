package resource

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

type KubeConformValidationError struct {
	Errors []error
}

// Error for interface compatibility
func (e KubeConformValidationError) Error() string {
	err := util.Multierrorf("validation error", e.Errors)
	if err != nil {
		return err.Error()
	}

	return "validation error"
}

// Can be called even without cluster access.
func ValidateLocal(ctx context.Context, releaseNamespace string, transformedResources []*InstallableResource,
	opts common.ResourceValidationOptions,
) error {
	if err := validateNoDuplicates(releaseNamespace, transformedResources); err != nil {
		return fmt.Errorf("validate for no duplicated resources: %w", err)
	}

	if featgate.FeatGateResourceValidation.Enabled() && !opts.NoResourceValidation {
		if err := validateResourceSchemas(ctx, releaseNamespace, transformedResources, opts); err != nil {
			return fmt.Errorf("validate resource schemas: %w", err)
		}
	}

	return nil
}

func validateResourceSchemas(ctx context.Context, releaseNamespace string, resources []*InstallableResource, opts common.ResourceValidationOptions) error {
	if len(resources) == 0 {
		return nil
	}

	resValidator, err := getKubeConformValidator(ctx, opts.ValidationKubeVersion)
	if err != nil {
		return fmt.Errorf("get schema validator: %w", err)
	}

	var errs []error

	for _, res := range resources {
		if ok, err := shouldSkipValidation(ctx, opts.ValidationSkip, releaseNamespace, res.ResourceMeta); err != nil {
			return fmt.Errorf("skip validation: %w", err)
		} else if ok {
			log.Default.Debug(ctx, "Skip local validation for resource (due to ValidationSkip): %s", res.IDHuman())

			continue
		}

		if err := validateResourceSchemaWithKubeConform(ctx, resValidator, res); err != nil {
			var kubeConformValidationError *KubeConformValidationError

			if errors.As(err, &kubeConformValidationError) {
				errs = append(errs, kubeConformValidationError.Errors...)

				continue
			}

			return fmt.Errorf("validate resource %s: %w", res.IDHuman(), err)
		}

		if err := validateResourceWithCodec(res); err != nil {
			errs = append(errs, fmt.Errorf("validate resource %s: %w", res.IDHuman(), err))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return util.Multierrorf("schema validation failed", errs)
}

func getKubeConformValidator(ctx context.Context, kubeVersion string) (validator.Validator, error) {
	kubeVersion = strings.TrimLeft(kubeVersion, "v")

	if err := checkIfKubeConformSpecExists(ctx, kubeVersion); err != nil {
		return nil, err
	}

	cacheDir, err := createKubeConformCacheDir(kubeVersion)
	if err != nil {
		return nil, fmt.Errorf("get schema cache dir: %w", err)
	}

	validatorOpts := validator.Opts{
		Strict:               false, // Skip undefined params check
		IgnoreMissingSchemas: true,
		Cache:                cacheDir,
		KubernetesVersion:    kubeVersion,
	}

	if log.Default.AcceptLevel(ctx, log.DebugLevel) {
		validatorOpts.Debug = true
	}

	validatorInstance, err := validator.New(common.APIResourceValidationKubeConformSchemasLocation, validatorOpts)
	if err != nil {
		return nil, fmt.Errorf("create schema validator: %w", err)
	}

	return validatorInstance, nil
}

func checkIfKubeConformSpecExists(ctx context.Context, kubeVersion string) error {
	// This file must exist to specific version. If it does not - version is not valid.
	url := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/v" + kubeVersion + "-standalone/all.json"

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("connect to download schemas: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download schemas: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("schemas for kube version %s not found", kubeVersion)
	}

	return nil
}

func createKubeConformCacheDir(version string) (string, error) {
	path := filepath.Join(common.APIResourceValidationJSONSchemasCacheDir, version)

	if stat, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", fmt.Errorf("create cache dir %q: %w", path, err)
		}

		return path, nil
	} else if err != nil {
		return "", fmt.Errorf("stat cache dir %q: %w", path, err)
	} else if !stat.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}

	return path, nil
}

func shouldSkipValidation(ctx context.Context, filters []string, releaseNamespace string, meta *spec.ResourceMeta) (bool, error) {
	for _, filter := range filters {
		properties := lo.Must(util.ParseProperties(ctx, filter))

		var matcher spec.ResourceMatcher

		for property, value := range properties {
			valueString := value.(string)

			switch property {
			case "group":
				matcher.Groups = append(matcher.Groups, valueString)
			case "version":
				matcher.Versions = append(matcher.Versions, valueString)
			case "kind":
				matcher.Kinds = append(matcher.Kinds, valueString)
			case "namespace":
				if valueString == "" {
					valueString = releaseNamespace
				}

				matcher.Namespaces = append(matcher.Namespaces, valueString)
			case "name":
				matcher.Names = append(matcher.Names, valueString)
			}
		}

		if matcher.Match(meta) {
			return true, nil
		}
	}

	return false, nil
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

func validateResourceSchemaWithKubeConform(ctx context.Context, kubeConformValidator validator.Validator, res *InstallableResource) error {
	yamlBytes, err := yaml.Marshal(res.Unstruct.Object)
	if err != nil {
		return fmt.Errorf("marshal resource to yaml: %w", err)
	}

	resCh, errCh := resource.FromStream(ctx, res.FilePath, bytes.NewReader(yamlBytes))

	var validationErrs []error

	for validationResource := range resCh {
		validationResult := kubeConformValidator.ValidateResource(validationResource)

		switch validationResult.Status {
		case validator.Error:
			return validationResult.Err
		case validator.Skipped:
			log.Default.Debug(ctx, "Skip validation for resource: %s", res.IDHuman())
		case validator.Invalid:
			for _, validationError := range validationResult.ValidationErrors {
				validationErrs = append(validationErrs,
					fmt.Errorf("validation %s: %s: %s", res.IDHuman(), validationError.Path, validationError.Msg))
			}
		case validator.Empty, validator.Valid:
			continue
		default:
			panic("unexpected validation status")
		}
	}

	// Check for stream reading Errors
	for err := range errCh {
		if err != nil {
			return fmt.Errorf("read resource stream: %w", err)
		}
	}

	if len(validationErrs) > 0 {
		return &KubeConformValidationError{Errors: validationErrs}
	}

	return nil
}

func validateResourceWithCodec(res *InstallableResource) error {
	unstruct := res.Unstruct
	gvk := unstruct.GroupVersionKind()

	jsonBytes, err := unstruct.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	decoder := scheme.Codecs.UniversalDecoder(gvk.GroupVersion())

	if _, _, err = decoder.Decode(jsonBytes, &gvk, nil); err != nil &&
		!strings.Contains(err.Error(), fmt.Sprintf("no kind %q is registered", res.GroupVersionKind.Kind)) {
		return fmt.Errorf("decoder decode: %w", err)
	}

	return nil
}
