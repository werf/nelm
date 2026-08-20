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
	PatchTypeJQ PatchType = "jq"

	// patchesFileName is the conventional name of a chart-shipped patches file.
	patchesFileName = "patches.yaml"
)

// PatchType is the transform kind of patch.
type PatchType string

// PatchesFile is the on-disk format of a patches file.
type PatchesFile struct {
	DiffPatches   []Patch `json:"diffPatches,omitempty"`
	RenderPatches []Patch `json:"renderPatches,omitempty"`
}

// Patches are patch rules grouped by the point at which they are applied.
type Patches struct {
	// Diff rules affect ONLY drift detection: the transform is applied identically
	// to the live and the dry-apply object before they are compared, so
	// normalized-away fields never produce a diff. They never change what is
	// rendered or applied.
	Diff []Patch
	// Render rules are applied to the rendered resources, so they do change what is
	// released and applied to the cluster.
	Render []Patch
}

// CompiledPatches are Patches with their jq programs compiled.
type CompiledPatches struct {
	Diff   []*CompiledPatch
	Render []*CompiledPatch
}

// Patch is a jq transform applied to every resource its matcher matches.
type Patch struct {
	// Match chooses which resources this rule applies to. An empty matcher
	// matches every resource.
	Match ResourceMatcher `json:"match,omitempty"`
	// Type is the transform kind. Only "jq" is supported; empty defaults to "jq".
	Type PatchType `json:"type,omitempty"`
	// Patch is the jq program: it receives the whole raw resource object and must
	// return exactly one object.
	Patch string `json:"patch,omitempty"`
	// ChartScope, when set, constrains this rule to resources originating in the
	// given chart subtree (a FilePath prefix). Set internally for chart-shipped
	// rules; never serialized or set by users.
	ChartScope string `json:"-"`
}

// CompiledPatch is a Patch with its jq program compiled once, ready to match and
// transform many resources.
type CompiledPatch struct {
	chartScope string
	code       *gojq.Code
	matcher    ResourceMatcher
}

// Match reports whether the rule matches the resource. namespace is the
// resource's true namespace (empty only for cluster-scoped resources), passed in
// because ResourceMeta.Namespace is blanked for release-namespace resources.
func (c *CompiledPatch) Match(resMeta *ResourceMeta, namespace string) bool {
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
func (c *CompiledPatch) transform(unstruct *unstructured.Unstructured) (result *unstructured.Unstructured, err error) {
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

// ApplyPatches runs every rule whose matcher matches the resource, threading each
// transform's output into the next, and returns a transformed deep copy; the
// input is never mutated. namespace is the resource's true namespace.
func ApplyPatches(patches []*CompiledPatch, resMeta *ResourceMeta, namespace string, unstruct *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	result := unstruct
	transformed := false

	for i, patch := range patches {
		if !patch.Match(resMeta, namespace) {
			continue
		}

		out, err := patch.transform(result)
		if err != nil {
			return nil, fmt.Errorf("apply patch #%d: %w", i+1, err)
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
func CollectChartPatches(chart helmchart.Accessor) (Patches, error) {
	if chart == nil {
		return Patches{}, nil
	}

	chartPath := chart.ChartFullPath()

	var patches Patches

	for _, dep := range chart.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return Patches{}, fmt.Errorf("access subchart of %q: %w", chartPath, err)
		}

		depPatches, err := CollectChartPatches(depAccessor)
		if err != nil {
			return Patches{}, err
		}

		patches.Diff = append(patches.Diff, depPatches.Diff...)
		patches.Render = append(patches.Render, depPatches.Render...)
	}

	var own Patches
	for _, f := range chart.Files() {
		if f.Name != patchesFileName {
			continue
		}

		ownPatches, err := parsePatchesFile(f.Data)
		if err != nil {
			return Patches{}, fmt.Errorf("read %s of chart %q: %w", patchesFileName, chartPath, err)
		}

		own = ownPatches

		break
	}

	for i := range own.Diff {
		own.Diff[i].ChartScope = chartPath
	}

	for i := range own.Render {
		own.Render[i].ChartScope = chartPath
	}

	patches.Diff = append(patches.Diff, own.Diff...)
	patches.Render = append(patches.Render, own.Render...)

	return patches, nil
}

// CompilePatches compiles patch rules, returning an error on the first invalid
// regexp, unsupported type, empty patch body, or invalid jq program.
func CompilePatches(patches []Patch) ([]*CompiledPatch, error) {
	if len(patches) == 0 {
		return nil, nil
	}

	compiled := make([]*CompiledPatch, 0, len(patches))
	for i, patch := range patches {
		c, err := compilePatch(patch)
		if err != nil {
			return nil, fmt.Errorf("compile patch #%d: %w", i+1, err)
		}

		compiled = append(compiled, c)
	}

	return compiled, nil
}

// LoadPatchesFiles reads and parses the given patches file paths, returning their
// rules concatenated in order.
func LoadPatchesFiles(paths []string) (Patches, error) {
	var patches Patches
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return Patches{}, fmt.Errorf("read patches file %q: %w", path, err)
		}

		filePatches, err := parsePatchesFile(data)
		if err != nil {
			return Patches{}, fmt.Errorf("patches file %q: %w", path, err)
		}

		patches.Diff = append(patches.Diff, filePatches.Diff...)
		patches.Render = append(patches.Render, filePatches.Render...)
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

func compilePatch(patch Patch) (*CompiledPatch, error) {
	patchType := patch.Type
	if patchType == "" {
		patchType = PatchTypeJQ
	}

	if patchType != PatchTypeJQ {
		return nil, fmt.Errorf("unsupported patch type %q, only %q is supported", patch.Type, PatchTypeJQ)
	}

	if strings.TrimSpace(patch.Patch) == "" {
		return nil, fmt.Errorf("patch program is empty")
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

	return &CompiledPatch{chartScope: patch.ChartScope, code: code, matcher: patch.Match}, nil
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

// parsePatchesFile parses a patches file into its patch rules. Unknown top-level
// keys are rejected so typos and unsupported kinds fail loudly.
func parsePatchesFile(data []byte) (Patches, error) {
	var file PatchesFile
	if err := yaml.UnmarshalStrict(data, &file); err != nil {
		return Patches{}, fmt.Errorf("parse patches file: %w", err)
	}

	return Patches{Diff: file.DiffPatches, Render: file.RenderPatches}, nil
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
