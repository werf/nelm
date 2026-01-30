package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/yannh/kubeconform/pkg/validator"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

var errDecode = errors.New("decode")

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

	kubeConformValidator, err := newKubeConformValidator(
		opts.ValidationKubeVersion,
		opts.ValidationSchemaCacheLifetime,
		append(opts.ValidationExtraSchemas, opts.ValidationSchemas...))
	if err != nil {
		return fmt.Errorf("get schema validator: %w", err)
	}

	validationErrs := &util.MultiError{}

	for _, res := range resources {
		if ok, err := shouldSkipValidation(ctx, opts.ValidationSkip, releaseNamespace, res.ResourceMeta); err != nil {
			return fmt.Errorf("skip validation: %w", err)
		} else if ok {
			log.Default.Debug(ctx, "Skip local validation for resource (due to ValidationSkip): %s", res.IDHuman())

			continue
		}

		if !opts.LocalResourceValidation {
			if err := kubeConformValidator.Validate(ctx, res.ResourceSpec); err != nil {
				e := fmt.Errorf("validate %s: %w", res.IDHuman(), err)

				var vErr *validator.ValidationError
				if errors.As(err, &vErr) {
					validationErrs.Add(e)

					continue
				}

				return e
			}
		}

		if err := validateResourceWithCodec(res); err != nil {
			e := fmt.Errorf("validate %s: %w", res.IDHuman(), err)

			if errors.Is(err, errDecode) {
				validationErrs.Add(e)

				continue
			}

			return e
		}
	}

	return validationErrs.OrNilIfNoErrs()
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

func validateResourceWithCodec(res *InstallableResource) error {
	unstruct := res.Unstruct
	gvk := unstruct.GroupVersionKind()

	jsonBytes, err := unstruct.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	decoder := scheme.Codecs.UniversalDecoder(gvk.GroupVersion())

	if _, _, err = decoder.Decode(jsonBytes, &gvk, nil); err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("no kind %q is registered", res.GroupVersionKind.Kind)) {
			return nil
		}

		return fmt.Errorf("%w: %w", errDecode, err)
	}

	return nil
}
