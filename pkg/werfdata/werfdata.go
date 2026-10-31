// Package werfdata reads and writes the .werf-data directory of a chart: the publish-time
// inputs (values, secret values, --set* flags, patches, extra values, annotations, labels,
// release defaults) that werf bundle publish and nelm chart pack store next to the chart, so
// that deploying the published chart renders like deploying the source chart with the same
// flags.
package werfdata

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/samber/lo"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
)

const (
	// APIVersionV2 is the only .werf-data format version. Charts without .werf-data are the v1
	// format of werf <= 3.12 bundles.
	APIVersionV2         = "v2"
	DirName              = ".werf-data"
	ExtraValuesFileName  = "extra-values.yaml"
	MetadataFileName     = "metadata.yaml"
	PatchesDirName       = "patches"
	SecretValuesFileName = "secret-values.yaml"
	SetFilesDirName      = "set-files"
	ValuesFileName       = "values.yaml"
)

// Metadata is the contents of .werf-data/metadata.yaml.
type Metadata struct {
	APIVersion              string            `json:"apiVersion"`
	Values                  ValuesMetadata    `json:"values"`
	Patches                 []string          `json:"patches,omitempty"`
	DefaultPatchesDisable   bool              `json:"defaultPatchesDisable,omitempty"`
	Release                 *ReleaseMetadata  `json:"release,omitempty"`
	Images                  []string          `json:"images,omitempty"`
	ExtraAnnotations        map[string]string `json:"extraAnnotations,omitempty"`
	ExtraLabels             map[string]string `json:"extraLabels,omitempty"`
	ExtraRuntimeAnnotations map[string]string `json:"extraRuntimeAnnotations,omitempty"`
	ReleaseInfoAnnotations  map[string]string `json:"releaseInfoAnnotations,omitempty"`
}

type ValuesMetadata struct {
	Values                     bool            `json:"values,omitempty"`
	SecretValues               bool            `json:"secretValues,omitempty"`
	ExtraValues                bool            `json:"extraValues,omitempty"`
	ValuesSet                  []string        `json:"valuesSet,omitempty"`
	ValuesSetString            []string        `json:"valuesSetString,omitempty"`
	ValuesSetJSON              []string        `json:"valuesSetJSON,omitempty"`
	ValuesSetLiteral           []string        `json:"valuesSetLiteral,omitempty"`
	ValuesSetFile              []SetFileRecord `json:"valuesSetFile,omitempty"`
	ValuesSetRootJSON          []string        `json:"valuesSetRootJSON,omitempty"`
	DefaultValuesDisable       bool            `json:"defaultValuesDisable,omitempty"`
	DefaultSecretValuesDisable bool            `json:"defaultSecretValuesDisable,omitempty"`
}

// SetFileRecord is one --set-file argument and the files it read at pack time. Contents maps
// each path in Value to a file under .werf-data/set-files.
type SetFileRecord struct {
	Value    string            `json:"value"`
	Contents map[string]string `json:"contents"`
}

type ReleaseMetadata struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Env       string `json:"env,omitempty"`
}

// Data is a parsed .werf-data directory.
type Data struct {
	// ExtraValues are the values werf generated at publish, nil if there were none.
	ExtraValues map[string]any
	// Files are the other files under .werf-data (set-files, patches), keyed by their path
	// relative to .werf-data.
	Files    map[string][]byte
	Metadata Metadata
	// SecretValues is the merged publish-time --secret-values, still encrypted, nil if there
	// were none.
	SecretValues []byte
	// Values is the merged publish-time --values, nil if there were none.
	Values map[string]any
}

// ChartFiles returns the .werf-data files, named relative to the chart root, in a stable order.
func (d *Data) ChartFiles() ([]*common.File, error) {
	metadata := d.Metadata
	metadata.APIVersion = APIVersionV2
	metadata.Values.Values = d.Values != nil
	metadata.Values.SecretValues = d.SecretValues != nil
	metadata.Values.ExtraValues = d.ExtraValues != nil

	metadataData, err := yaml.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", MetadataFileName, err)
	}

	byName := map[string][]byte{MetadataFileName: metadataData}

	if d.Values != nil {
		valuesData, err := yaml.Marshal(d.Values)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", ValuesFileName, err)
		}

		byName[ValuesFileName] = valuesData
	}

	if d.SecretValues != nil {
		byName[SecretValuesFileName] = d.SecretValues
	}

	if d.ExtraValues != nil {
		extraValuesData, err := yaml.Marshal(d.ExtraValues)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", ExtraValuesFileName, err)
		}

		byName[ExtraValuesFileName] = extraValuesData
	}

	for name, fileData := range d.Files {
		byName[name] = fileData
	}

	names := lo.Keys(byName)
	sort.Strings(names)

	return lo.Map(names, func(name string, _ int) *common.File {
		return &common.File{Name: path.Join(DirName, name), Data: byName[name]}
	}), nil
}

// PatchesFiles returns the stored publish-time patches files, in order.
func (d *Data) PatchesFiles() ([][]byte, error) {
	var result [][]byte
	for _, name := range d.Metadata.Patches {
		fileData, ok := d.Files[name]
		if !ok {
			return nil, fmt.Errorf("%s/%s not found", DirName, name)
		}

		result = append(result, fileData)
	}

	return result, nil
}

// SetFileContents returns the stored contents of a --set-file record, keyed by the path as
// given at pack time.
func (d *Data) SetFileContents(record SetFileRecord) (map[string]string, error) {
	contents := map[string]string{}
	for filePath, storedName := range record.Contents {
		fileData, ok := d.Files[storedName]
		if !ok {
			return nil, fmt.Errorf("%s/%s not found for --set-file %q", DirName, storedName, record.Value)
		}

		contents[filePath] = string(fileData)
	}

	return contents, nil
}

func (d *Data) validate() error {
	if d.Metadata.Values.Values != (d.Values != nil) {
		return fmt.Errorf("%s/%s: values: %t does not match the presence of %s", DirName, MetadataFileName, d.Metadata.Values.Values, ValuesFileName)
	}

	if d.Metadata.Values.SecretValues != (d.SecretValues != nil) {
		return fmt.Errorf("%s/%s: secretValues: %t does not match the presence of %s", DirName, MetadataFileName, d.Metadata.Values.SecretValues, SecretValuesFileName)
	}

	if d.Metadata.Values.ExtraValues != (d.ExtraValues != nil) {
		return fmt.Errorf("%s/%s: extraValues: %t does not match the presence of %s", DirName, MetadataFileName, d.Metadata.Values.ExtraValues, ExtraValuesFileName)
	}

	for _, record := range d.Metadata.Values.ValuesSetFile {
		for _, storedName := range record.Contents {
			if !strings.HasPrefix(storedName, SetFilesDirName+"/") {
				return fmt.Errorf("%s/%s: --set-file %q points outside of %s: %q", DirName, MetadataFileName, record.Value, SetFilesDirName, storedName)
			}

			if _, ok := d.Files[storedName]; !ok {
				return fmt.Errorf("%s/%s not found for --set-file %q", DirName, storedName, record.Value)
			}
		}
	}

	for _, name := range d.Metadata.Patches {
		if !strings.HasPrefix(name, PatchesDirName+"/") {
			return fmt.Errorf("%s/%s: patches file points outside of %s: %q", DirName, MetadataFileName, PatchesDirName, name)
		}

		if _, ok := d.Files[name]; !ok {
			return fmt.Errorf("%s/%s not found", DirName, name)
		}
	}

	return nil
}

// IsWerfDataFileInTree reports whether a chart file path, relative to the chart root, belongs to
// the .werf-data of the chart or of a dependency unpacked under charts/, at any depth.
func IsWerfDataFileInTree(name string) bool {
	for !IsWerfDataFile(name) {
		rest, ok := strings.CutPrefix(name, "charts/")
		if !ok {
			return false
		}

		if _, name, ok = strings.Cut(rest, "/"); !ok {
			return false
		}
	}

	return true
}

// Parse parses the .werf-data files of a chart. Names are relative to the chart root. A chart
// without .werf-data files has no .werf-data: check for that before calling Parse.
func Parse(files []*common.File) (*Data, error) {
	byName := map[string][]byte{}
	for _, f := range files {
		if !IsWerfDataFile(f.Name) {
			return nil, fmt.Errorf("file %q is not in %s", f.Name, DirName)
		}

		byName[strings.TrimPrefix(f.Name, DirName+"/")] = f.Data
	}

	metadataData, ok := byName[MetadataFileName]
	if !ok {
		return nil, fmt.Errorf("%s/%s not found", DirName, MetadataFileName)
	}

	data := &Data{Files: map[string][]byte{}}
	if err := yaml.UnmarshalStrict(metadataData, &data.Metadata); err != nil {
		return nil, fmt.Errorf("parse %s/%s: %w", DirName, MetadataFileName, err)
	}

	if err := CheckAPIVersion(data.Metadata.APIVersion); err != nil {
		return nil, err
	}

	for name, fileData := range byName {
		switch name {
		case MetadataFileName:
		case ValuesFileName:
			vals, err := parseValues(name, fileData)
			if err != nil {
				return nil, err
			}

			data.Values = vals
		case SecretValuesFileName:
			data.SecretValues = fileData
		case ExtraValuesFileName:
			vals, err := parseValues(name, fileData)
			if err != nil {
				return nil, err
			}

			data.ExtraValues = vals
		default:
			data.Files[name] = fileData
		}
	}

	if err := data.validate(); err != nil {
		return nil, err
	}

	return data, nil
}

// CheckAPIVersion fails for a .werf-data apiVersion this version doesn't know.
func CheckAPIVersion(apiVersion string) error {
	if apiVersion == APIVersionV2 {
		return nil
	}

	return fmt.Errorf("unsupported %s/%s apiVersion %q: this chart requires a newer werf or Nelm", DirName, MetadataFileName, apiVersion)
}

// IsWerfDataFile reports whether a chart file path, relative to the chart root, belongs to
// .werf-data.
func IsWerfDataFile(name string) bool {
	return name == DirName || strings.HasPrefix(name, DirName+"/")
}

func parseValues(name string, data []byte) (map[string]any, error) {
	vals := map[string]any{}
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil, fmt.Errorf("parse %s/%s: %w", DirName, name, err)
	}

	return vals, nil
}
