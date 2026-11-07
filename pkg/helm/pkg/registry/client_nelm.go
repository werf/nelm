package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
)

// PushOptExtraTags returns a function that sets additional tags pointing to the
// pushed manifest. Plus signs are replaced with underscores, as in the main ref.
// Strict mode applies only to the main ref.
func PushOptExtraTags(tags []string) PushOption {
	return func(operation *pushOperation) {
		operation.extraTags = tags
	}
}

func normalizeExtraTags(parsedRef reference, tags []string) ([]string, error) {
	var result []string
	for _, tag := range tags {
		tag = strings.ReplaceAll(tag, "+", "_")
		if tag == parsedRef.Tag || slices.Contains(result, tag) {
			continue
		}

		tagRef := parsedRef.orasReference
		tagRef.Reference = tag
		if err := tagRef.ValidateReferenceAsTag(); err != nil {
			return nil, fmt.Errorf("validate extra tag: %w", err)
		}

		if err := validateChartTag(tag); err != nil {
			return nil, err
		}

		result = append(result, tag)
	}

	return result, nil
}

// The OCI getter treats a ref ending in ".prov" as a request for the provenance of the chart under the rest of the ref.
func validateChartTag(tag string) error {
	if strings.HasSuffix(tag, ".prov") {
		return fmt.Errorf("tag %q is not allowed: tags ending in \".prov\" address provenance files", tag)
	}

	return nil
}

func (c *Client) pushExtraTags(ctx context.Context, repository *remote.Repository, manifestDescriptor ocispec.Descriptor, parsedRef reference, pushedRef string, extraTags []string) error {
	for _, tag := range extraTags {
		if err := repository.Tag(ctx, manifestDescriptor, tag); err != nil {
			return fmt.Errorf("chart pushed as %s, but tagging it also as %q failed: %w", pushedRef, tag, err)
		}
		_, _ = fmt.Fprintf(c.out, "Tagged: %s:%s\n", parsedRef.Registry+"/"+parsedRef.Repository, tag)
	}

	return nil
}

func (c *Client) tagExists(registryReference reference, tag string) (bool, error) {
	tagReference := registryReference.orasReference
	tagReference.Reference = tag
	if tagReference.ValidateReferenceAsTag() != nil {
		return false, nil
	}

	if _, err := c.Resolve(tagReference.String()); err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}
