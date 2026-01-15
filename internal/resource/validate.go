package resource

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

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
	var errs []string

	for _, res := range transformedResources {
		if shouldSkipSchemaValidation(res) {
			continue
		}

		if err := validateResourceSchema(res); err != nil {
			errs = append(errs, "\tvalidate "+res.IDHuman()+": "+err.Error())
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("schema validation failed:\n%s", strings.Join(errs, "\n"))
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

// validateResourceSchema validates a single resource by attempting to decode it into a typed object.
// This will catch schema mismatches, invalid field values, and missing required fields.
func validateResourceSchema(res *InstallableResource) error {
	unstruct := res.Unstruct
	gvk := unstruct.GroupVersionKind()

	originalUnstruct := unstruct.DeepCopy()

	jsonBytes, err := originalUnstruct.MarshalJSON()
	if err != nil {
		return err
	}

	decoder := scheme.Codecs.UniversalDecoder(gvk.GroupVersion())

	// Decode into typed object - this validates the schema
	obj, _, err := decoder.Decode(jsonBytes, &gvk, nil)
	if err != nil {
		return err
	}

	// Convert typed object back to unstructured
	// This will drop any unknown fields that weren't in the schema
	convertedUnstruct, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}

	unknownFields := findUnknownFields(originalUnstruct.Object, convertedUnstruct, "")
	if len(unknownFields) > 0 {
		return fmt.Errorf("unknown fields found: %s", strings.Join(unknownFields, ", "))
	}

	return nil
}

// findUnknownFields recursively compares the original and converted unstructured objects
// to find fields that exist in the original but not in the converted (unknown fields).
func findUnknownFields(original map[string]interface{}, converted map[string]interface{}, pathPrefix string) []string {
	var unknownFields []string

	for key, originalValue := range original {
		// Skip metadata fields that are always present and may be normalized
		if pathPrefix == "" && (key == "apiVersion" || key == "kind" || key == "metadata") {
			continue
		}

		convertedValue, exists := converted[key]
		if !exists {
			fieldPath := key

			if pathPrefix != "" {
				fieldPath = pathPrefix + "." + key
			}

			unknownFields = append(unknownFields, fieldPath)

			continue
		}

		originalMap, originalIsMap := originalValue.(map[string]interface{})
		convertedMap, convertedIsMap := convertedValue.(map[string]interface{})

		if originalIsMap && convertedIsMap {
			fieldPath := key

			if pathPrefix != "" {
				fieldPath = pathPrefix + "." + key
			}

			nestedUnknown := findUnknownFields(originalMap, convertedMap, fieldPath)
			unknownFields = append(unknownFields, nestedUnknown...)
		}
	}

	return unknownFields
}
