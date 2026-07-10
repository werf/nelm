package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/itchyny/gojq"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	helmchart "github.com/werf/nelm/pkg/helm/pkg/chart"
)

const (
	DiffPatchTypeJQ DiffPatchType = "jq"

	// patchesFileName is the conventional name of a chart-shipped patches file.
	patchesFileName = "patches.yaml"
)

// DiffPatchType is the transform kind of a diff patch.
type DiffPatchType string

// PatchesFile is the on-disk format of a patches file. Only diffPatches is
// supported today; the top-level key leaves room for future siblings.
type PatchesFile struct {
	DiffPatches []DiffPatch `json:"diffPatches,omitempty"`
}

// DiffPatch is a diff-time normalization rule. It affects ONLY drift detection:
// the transform is applied identically to the live and the dry-apply object
// before they are compared, so normalized-away fields never produce a diff.
type DiffPatch struct {
	// Match chooses which resources this rule applies to. An empty matcher
	// matches every resource.
	Match ResourceMatcher `json:"match,omitempty"`
	// Type is the transform kind. Only "jq" is supported; empty defaults to "jq".
	Type DiffPatchType `json:"type,omitempty"`
	// Patch is the jq program: it receives the whole raw resource object and must
	// return exactly one object.
	Patch string `json:"patch,omitempty"`
	// ChartScope, when set, constrains this rule to resources originating in the
	// given chart subtree (a FilePath prefix). Set internally for chart-shipped
	// rules; never serialized or set by users.
	ChartScope string `json:"-"`
}

// CompiledDiffPatch is a DiffPatch with its jq program compiled once, ready to
// match and transform many resources.
type CompiledDiffPatch struct {
	chartScope string
	code       *gojq.Code
	matcher    ResourceMatcher
}

// Match reports whether the rule matches the resource. namespace is the
// resource's true namespace (empty only for cluster-scoped resources), passed in
// because ResourceMeta.Namespace is blanked for release-namespace resources.
func (c *CompiledDiffPatch) Match(resMeta *ResourceMeta, namespace string) bool {
	if !resourceInChartScope(c.chartScope, resMeta.FilePath) {
		return false
	}

	// Match against the resource's true namespace, not the blanked meta value.
	scoped := *resMeta
	scoped.Namespace = namespace

	return c.matcher.Match(&scoped)
}

// transform runs the compiled jq program over a deep copy of the object and
// returns the single object output. Zero, multiple, or non-object output is an
// error, and a jq panic is recovered into an error; the input is never mutated.
func (c *CompiledDiffPatch) transform(unstruct *unstructured.Unstructured) (result *unstructured.Unstructured, err error) {
	// Unstructured stores integers as int64, which gojq rejects; round-trip
	// through JSON with UseNumber so numbers reach gojq as json.Number.
	input, err := toJQInput(unstruct.Object)
	if err != nil {
		return nil, fmt.Errorf("normalize resource for jq: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("jq program panicked: %v", r)
		}
	}()

	iter := c.code.Run(input)

	first, ok := iter.Next()
	if !ok {
		return nil, fmt.Errorf("jq program produced no output, want exactly one object")
	}

	if err, ok := first.(error); ok {
		return nil, fmt.Errorf("run jq program: %w", err)
	}

	if _, ok := iter.Next(); ok {
		return nil, fmt.Errorf("jq program produced more than one output, want exactly one object")
	}

	if _, ok := first.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("jq program output is %T, want an object", first)
	}

	obj, err := fromJQOutput(first)
	if err != nil {
		return nil, fmt.Errorf("convert jq output for resource: %w", err)
	}

	return &unstructured.Unstructured{Object: obj}, nil
}

// ApplyDiffPatches runs every rule whose matcher matches the resource, threading
// each transform's output into the next, and returns a transformed deep copy; the
// input is never mutated. namespace is the resource's true namespace.
func ApplyDiffPatches(patches []*CompiledDiffPatch, resMeta *ResourceMeta, namespace string, unstruct *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	result := unstruct
	transformed := false

	for i, patch := range patches {
		if !patch.Match(resMeta, namespace) {
			continue
		}

		out, err := patch.transform(result)
		if err != nil {
			return nil, fmt.Errorf("apply diff patch #%d: %w", i+1, err)
		}

		result = out
		transformed = true
	}

	if !transformed {
		return unstruct.DeepCopy(), nil
	}

	return result, nil
}

// CollectChartPatches returns every chart-shipped patches.yaml rule in the chart
// tree, ordered leaf-first and each constrained to its own chart subtree.
func CollectChartPatches(chart helmchart.Accessor) ([]DiffPatch, error) {
	if chart == nil {
		return nil, nil
	}

	chartPath := chart.ChartFullPath()

	var patches []DiffPatch

	for _, dep := range chart.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return nil, fmt.Errorf("access subchart of %q: %w", chartPath, err)
		}

		depPatches, err := CollectChartPatches(depAccessor)
		if err != nil {
			return nil, err
		}

		patches = append(patches, depPatches...)
	}

	var own []DiffPatch
	for _, f := range chart.Files() {
		if f.Name != patchesFileName {
			continue
		}

		ownPatches, err := parsePatchesFile(f.Data)
		if err != nil {
			return nil, fmt.Errorf("read %s of chart %q: %w", patchesFileName, chartPath, err)
		}

		own = ownPatches

		break
	}

	for i := range own {
		own[i].ChartScope = chartPath
	}

	return append(patches, own...), nil
}

// CompileDiffPatches compiles diff patch rules, returning a plan error on the
// first invalid regexp, unsupported type, empty patch body, or invalid jq
// program.
func CompileDiffPatches(patches []DiffPatch) ([]*CompiledDiffPatch, error) {
	if len(patches) == 0 {
		return nil, nil
	}

	compiled := make([]*CompiledDiffPatch, 0, len(patches))
	for i, patch := range patches {
		c, err := compileDiffPatch(patch)
		if err != nil {
			return nil, fmt.Errorf("compile diff patch #%d: %w", i+1, err)
		}

		compiled = append(compiled, c)
	}

	return compiled, nil
}

// LoadPatchesFiles reads and parses the given patches file paths, returning their
// rules concatenated in order.
func LoadPatchesFiles(paths []string) ([]DiffPatch, error) {
	var patches []DiffPatch
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read patches file %q: %w", path, err)
		}

		filePatches, err := parsePatchesFile(data)
		if err != nil {
			return nil, fmt.Errorf("patches file %q: %w", path, err)
		}

		patches = append(patches, filePatches...)
	}

	return patches, nil
}

func fromJQOutput(value interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var decoded interface{}
	if err := dec.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	normalized, ok := normalizeNumbers(decoded).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("jq program output is not an object")
	}

	return normalized, nil
}

func compileDiffPatch(patch DiffPatch) (*CompiledDiffPatch, error) {
	patchType := patch.Type
	if patchType == "" {
		patchType = DiffPatchTypeJQ
	}

	if patchType != DiffPatchTypeJQ {
		return nil, fmt.Errorf("unsupported diff patch type %q, only %q is supported", patch.Type, DiffPatchTypeJQ)
	}

	if strings.TrimSpace(patch.Patch) == "" {
		return nil, fmt.Errorf("diff patch program is empty")
	}

	if err := patch.Match.Validate(); err != nil {
		return nil, fmt.Errorf("invalid matcher: %w", err)
	}

	query, err := gojq.Parse(patch.Patch)
	if err != nil {
		return nil, fmt.Errorf("parse jq program: %w", err)
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("compile jq program: %w", err)
	}

	return &CompiledDiffPatch{chartScope: patch.ChartScope, code: code, matcher: patch.Match}, nil
}

func normalizeNumbers(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, elem := range v {
			v[key] = normalizeNumbers(elem)
		}

		return v
	case []interface{}:
		for i, elem := range v {
			v[i] = normalizeNumbers(elem)
		}

		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}

		if f, err := v.Float64(); err == nil {
			return f
		}

		return v.String()
	default:
		return value
	}
}

// parsePatchesFile parses a patches file into its diff patch rules. Unknown
// top-level keys are rejected so typos and unsupported kinds fail loudly.
func parsePatchesFile(data []byte) ([]DiffPatch, error) {
	var file PatchesFile
	if err := yaml.UnmarshalStrict(data, &file); err != nil {
		return nil, fmt.Errorf("parse patches file: %w", err)
	}

	return file.DiffPatches, nil
}

func resourceInChartScope(chartPath, filePath string) bool {
	if chartPath == "" {
		return true
	}

	return filePath == chartPath || strings.HasPrefix(filePath, chartPath+"/")
}

func toJQInput(obj map[string]interface{}) (interface{}, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var out interface{}
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return out, nil
}
