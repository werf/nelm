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

	"github.com/werf/3p-helm/pkg/helmpath"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

// Can be called even without cluster access.
func ValidateLocal(ctx context.Context, releaseNamespace string, transformedResources []*InstallableResource,
	opts common.ResourceLocalValidationOptions,
) error {
	if err := validateNoDuplicates(releaseNamespace, transformedResources); err != nil {
		return fmt.Errorf("validate for no duplicated resources: %w", err)
	}

	if !featgate.FeatGateLocalResourceValidation.Enabled() || opts.NoResourceValidation {
		return nil
	}

	if err := validateResourceSchemas(ctx, releaseNamespace, transformedResources, opts); err != nil {
		return fmt.Errorf("validate resource schemas: %w", err)
	}

	return nil
}

func validateResourceSchemas(ctx context.Context, releaseNamespace string, resources []*InstallableResource, opts common.ResourceLocalValidationOptions) error {
	if len(resources) == 0 {
		return nil
	}

	resValidator, err := getKubeConformValidator(ctx, opts.ValidationKubeVersion)
	if err != nil {
		return fmt.Errorf("get schema validator: %w", err)
	}

	var sb strings.Builder

	for _, res := range resources {
		if ok, err := shouldSkipValidation(ctx, opts.ValidationSkip, releaseNamespace, res.ResourceMeta); err != nil {
			return fmt.Errorf("skip validation: %w", err)
		} else if ok {
			log.Default.Debug(ctx, "Skip local validation for resource (due to ValidationSkip): %s", res.IDHuman())

			continue
		}

		if err := validateResourceSchemaWithKubeConform(ctx, resValidator, res); err != nil {
			sb.WriteString("  validate " + res.IDHuman() + ": " + err.Error() + "\n")

			continue
		}

		if err := validateResourceWithCodec(res); err != nil &&
			!strings.Contains(err.Error(), fmt.Sprintf("no kind %q is registered", res.GroupVersionKind.Kind)) {
			sb.WriteString("  validate " + res.IDHuman() + ": " + err.Error() + "\n")
		}
	}

	if sb.Len() == 0 {
		return nil
	}

	return fmt.Errorf("schema validation failed:\n%s", sb.String())
}

func getKubeConformValidator(ctx context.Context, kubeVersion string) (validator.Validator, error) {
	kubeVersion = strings.TrimLeft(kubeVersion, "v")

	if err := checkIfKubeConformSpecExists(ctx, kubeVersion); err != nil {
		return nil, err
	}

	schemaLocations := []string{
		"default",
		"https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json",
	}

	cacheDir, err := getKubeConformSchemaCacheDir(kubeVersion, true)
	if err != nil {
		return nil, fmt.Errorf("get schema cache dir: %w", err)
	}

	validatorOpts := validator.Opts{
		Strict:               false, // Skip undefined params check
		IgnoreMissingSchemas: true,
		Cache:                cacheDir,
		KubernetesVersion:    kubeVersion,
		Debug:                true,
	}

	validatorInstance, err := validator.New(schemaLocations, validatorOpts)
	if err != nil {
		return nil, fmt.Errorf("create schema validator: %w", err)
	}

	return validatorInstance, nil
}

func checkIfKubeConformSpecExists(ctx context.Context, kubeVersion string) error {
	cacheDir, err := getKubeConformSchemaCacheDir(kubeVersion, false)
	if err != nil {
		return err
	}

	// Use signal file to avoid sending subsequent requests
	signalFilePath := filepath.Join(cacheDir, "ready")

	stat, err := os.Stat(signalFilePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check cache dir: %w", err)
	}

	if stat != nil {
		return nil
	}

	// This file must exist to specific version. If it does not - version is not valid.
	url := "https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/v" + kubeVersion + "-standalone/all.json"

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
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
		return errors.New("schemas not found")
	}

	_, err = getKubeConformSchemaCacheDir(kubeVersion, true)
	if err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	f, err := os.Create(signalFilePath)
	if err != nil {
		return fmt.Errorf("cannot initialize kubernetes %s cache: %w", signalFilePath, err)
	}

	defer f.Close()

	return nil
}

func getKubeConformSchemaCacheDir(kubeVersion string, create bool) (string, error) {
	cacheDirPath := helmpath.CachePath(".nelm", "api-resource-json-schemas", kubeVersion)

	if !create {
		return cacheDirPath, nil
	}

	if stat, err := os.Stat(cacheDirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDirPath, 0o750); err != nil {
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

func shouldSkipValidation(ctx context.Context, filters []string, releaseNamespace string, meta *spec.ResourceMeta) (bool, error) {
	metaMap := map[string]func(string) bool{
		"kind":    func(s string) bool { return s == meta.GroupVersionKind.Kind },
		"group":   func(s string) bool { return s == meta.GroupVersionKind.Group },
		"version": func(s string) bool { return s == meta.GroupVersionKind.Version },
		// TODO: need to figure out how to filter on namespace set in the manifest itself
		"namespace": func(s string) bool { return s == meta.Namespace || s == releaseNamespace },
		"name":      func(s string) bool { return s == meta.Name },
	}

INPUT:
	for _, filter := range filters {
		var match bool

		properties, err := util.ParseProperties(ctx, filter)
		if err != nil {
			return false, fmt.Errorf("cannot parse properties: %w", err)
		}

		for property := range properties {
			if _, found := metaMap[property]; !found {
				return false, fmt.Errorf("only %s skip properties are supported",
					strings.Join(lo.Keys(metaMap), ", "))
			}
		}

		for property, value := range properties {
			f := metaMap[property]

			if stringValue, ok := value.(string); !ok {
				return false, fmt.Errorf("property %s has invalid value", property)
			} else if !f(stringValue) {
				continue INPUT
			}

			match = true
		}

		if match {
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

func validateResourceSchemaWithKubeConform(ctx context.Context, kubeConformValidator validator.Validator,
	res *InstallableResource,
) error {
	yamlBytes, err := yaml.Marshal(res.Unstruct.Object)
	if err != nil {
		return fmt.Errorf("marshal resource to yaml: %w", err)
	}

	resCh, errCh := resource.FromStream(ctx, res.FilePath, bytes.NewReader(yamlBytes))

	var validationErrs []string

	for validationResource := range resCh {
		validationResult := kubeConformValidator.ValidateResource(validationResource)

		if validationResult.Status == validator.Error {
			return validationResult.Err
		}

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
		return fmt.Errorf("%s", strings.Join(validationErrs, "\n"))
	}

	return nil
}

func validateResourceWithCodec(res *InstallableResource) error {
	unstruct := res.Unstruct
	gvk := unstruct.GroupVersionKind()

	jsonBytes, err := unstruct.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal josn: %w", err)
	}

	decoder := scheme.Codecs.UniversalDecoder(gvk.GroupVersion())

	_, _, err = decoder.Decode(jsonBytes, &gvk, nil)
	if err != nil {
		return fmt.Errorf("decoder decode: %w", err)
	}

	return nil
}
