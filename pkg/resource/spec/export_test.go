package spec

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

var (
	// CompileDiffPatch and ParsePatchesFile expose unexported symbols to the
	// external spec_test package.
	CompileDiffPatch = compileDiffPatch
	ParsePatchesFile = parsePatchesFile
)

// Transform exposes the unexported transform method to the external spec_test package.
func (c *CompiledDiffPatch) Transform(unstruct *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.transform(unstruct)
}
