package spec

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	// CompilePatch and ParsePatchesFile expose unexported symbols to the
	// external spec_test package.
	CompilePatch     = compilePatch
	ParsePatchesFile = parsePatchesFile
)

// Transform exposes the unexported transform method to the external spec_test package.
func (c *CompiledPatch) Transform(ctx context.Context, unstruct *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.transform(ctx, unstruct)
}
