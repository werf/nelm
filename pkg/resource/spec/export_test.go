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

// ChartScope exposes the unexported chart scope to the external spec_test package.
func (p Patch) ChartScope() string {
	return p.chartScope
}

// Transform exposes the unexported transform method to the external spec_test package.
func (c *CompiledPatch) Transform(ctx context.Context, unstruct *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.transform(ctx, unstruct)
}

// NewChartScopedPatch builds a chart-scoped patch for the external spec_test package.
func NewChartScopedPatch(chartScope, patch string) Patch {
	return Patch{chartScope: chartScope, Patch: patch}
}
