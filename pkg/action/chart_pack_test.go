package action

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/secret"
	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/chartyaml"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/featgate"
	"github.com/werf/nelm/v2/pkg/helm/intern/resolver"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	v2loader "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/loader"
	v2util "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/provenance"
	repo "github.com/werf/nelm/v2/pkg/helm/pkg/repo/v1"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

const chartPackTestChartYAML = `# The chart.
apiVersion: v2   # chart api
name: mychart
# Bumped by CI.
version: 1.0.0
annotations:
  team: payments # owner
`

type chartPackTestStep struct {
	expectedChartYAML string
	// inputs are written into the working directory before the step.
	inputs map[string]string
	opts   ChartPackOptions
}

type chartPackTestFileReader struct {
	common.ChartFileReaderer

	files map[string]string
}

func (r chartPackTestFileReader) ReadChartFile(_ context.Context, filePath string) ([]byte, error) {
	data, ok := r.files[filePath]
	if !ok {
		return nil, os.ErrNotExist
	}

	return []byte(data), nil
}

func TestAddChartPackInputs_ReadsPatchesThroughChartFileReader(t *testing.T) {
	const patches = "renderPatches:\n- patch: del(.a)\n"

	chartFileReader := common.ChartFileReader
	common.ChartFileReader = chartPackTestFileReader{files: map[string]string{"committed/patches.yaml": patches}}

	t.Cleanup(func() {
		common.ChartFileReader = chartFileReader
	})

	data := &werfdata.Data{Files: map[string][]byte{}}
	require.NoError(t, addChartPackInputs(newChartPackTestContext(), data, ChartPackOptions{PatchesFiles: []string{"committed/patches.yaml"}}))
	assert.Equal(t, map[string][]byte{"patches/0": []byte(patches)}, data.Files)
}

func TestChartPack(t *testing.T) {
	encryptedPassword := encryptChartPackTestValues(t, "password: p1\nlist: [s1, s2]\n")
	encryptedList := encryptChartPackTestValues(t, "list: [s3]\n")

	tests := []struct {
		name                 string
		chart                map[string]string
		steps                []chartPackTestStep
		expectedData         *werfdata.Data
		expectedSecretValues map[string]any
		expectedFiles        []string
		expectedErr          string
	}{
		{
			name:  "fresh pack with all inputs",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{
				expectedChartYAML: `# The chart.
apiVersion: v2 # chart api
name: mychart
# Bumped by CI.
version: 1.0.0
annotations:
  team: payments # owner
  helm.sh/images: |
    - name: backend
      image: registry/app:backend
`,
				inputs: map[string]string{
					"values1.yaml": "a: one\nnested:\n  k: v\n  list: [x]\n",
					"values2.yaml": "nested:\n  k2: v2\n  list: [z]\n",
					"secret1.yaml": encryptedPassword,
					"secret2.yaml": encryptedList,
					"file1.txt":    "file one",
					"file2.txt":    "file two",
					"patches.yaml": "renderPatches:\n- patch: del(.a)\n",
				},
				opts: ChartPackOptions{
					DefaultValuesDisable:       lo.ToPtr(true),
					RootSetJSON:                []string{`r={"y":2}`},
					ValuesFiles:                []string{"values1.yaml", "values2.yaml"},
					ValuesSet:                  []string{"s=1"},
					ValuesSetFile:              []string{"f=file1.txt,g=file2.txt"},
					ValuesSetJSON:              []string{`j={"x":1}`},
					ValuesSetLiteral:           []string{"l=3"},
					ValuesSetString:            []string{"ss=2"},
					DefaultPatchesDisable:      lo.ToPtr(true),
					DefaultSecretValuesDisable: lo.ToPtr(true),
					ExtraAnnotations:           map[string]string{"team": "payments"},
					ExtraLabels:                map[string]string{"tier": "backend"},
					ExtraRuntimeAnnotations:    map[string]string{"project.werf.io/name": "myapp"},
					ExtraValues:                map[string]interface{}{"werf": map[string]interface{}{"env": "production"}},
					Images:                     []chartyaml.Image{{Name: "backend", Image: "registry/app:backend"}},
					PatchesFiles:               []string{"patches.yaml"},
					Release:                    &werfdata.ReleaseMetadata{Name: "myapp-production", Namespace: "myapp", Env: "production"},
					ReleaseInfoAnnotations:     map[string]string{"project.werf.io/name": "myapp"},
					SecretValuesFiles:          []string{"secret1.yaml", "secret2.yaml"},
				},
			}},
			expectedData: &werfdata.Data{
				ExtraValues: map[string]any{"werf": map[string]any{"env": "production"}},
				Files: map[string][]byte{
					"patches/0":   []byte("renderPatches:\n- patch: del(.a)\n"),
					"set-files/0": []byte("file one"),
					"set-files/1": []byte("file two"),
				},
				Metadata: werfdata.Metadata{
					APIVersion: werfdata.APIVersionV2,
					Values: werfdata.ValuesMetadata{
						Values:           true,
						SecretValues:     true,
						ExtraValues:      true,
						ValuesSet:        []string{"s=1"},
						ValuesSetString:  []string{"ss=2"},
						ValuesSetJSON:    []string{`j={"x":1}`},
						ValuesSetLiteral: []string{"l=3"},
						ValuesSetFile: []werfdata.SetFileRecord{{
							Value:    "f=file1.txt,g=file2.txt",
							Contents: map[string]string{"file1.txt": "set-files/0", "file2.txt": "set-files/1"},
						}},
						ValuesSetRootJSON:          []string{`r={"y":2}`},
						DefaultValuesDisable:       true,
						DefaultSecretValuesDisable: true,
					},
					Patches:                 []string{"patches/0"},
					DefaultPatchesDisable:   true,
					Release:                 &werfdata.ReleaseMetadata{Name: "myapp-production", Namespace: "myapp", Env: "production"},
					Images:                  []string{"backend"},
					ExtraAnnotations:        map[string]string{"team": "payments"},
					ExtraLabels:             map[string]string{"tier": "backend"},
					ExtraRuntimeAnnotations: map[string]string{"project.werf.io/name": "myapp"},
					ReleaseInfoAnnotations:  map[string]string{"project.werf.io/name": "myapp"},
				},
				Values: map[string]any{"a": "one", "nested": map[string]any{"k": "v", "k2": "v2", "list": []any{"z"}}},
			},
			expectedSecretValues: map[string]any{"password": "p1", "list": []any{"s3"}},
		},
		{
			name:  "no inputs, no .werf-data",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{expectedChartYAML: chartPackTestChartYAML, opts: ChartPackOptions{Images: []chartyaml.Image{}}}},
		},
		{
			name:  "repack adds set over stored values",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{
				{
					inputs: map[string]string{"values1.yaml": "a: one\nnested:\n  k: v\n"},
					opts: ChartPackOptions{
						ValuesFiles: []string{"values1.yaml"}, ValuesSet: []string{"s=1"},
						ExtraAnnotations: map[string]string{"team": "payments", "tier": "backend"},
						ExtraValues:      map[string]interface{}{"werf": "old"},
					},
				},
				{
					inputs: map[string]string{"values2.yaml": "nested:\n  k2: v2\n"},
					opts: ChartPackOptions{
						ValuesFiles: []string{"values2.yaml"}, ValuesSet: []string{"a=two"},
						ExtraAnnotations: map[string]string{"team": "billing"},
					},
				},
			},
			expectedData: &werfdata.Data{
				ExtraValues: map[string]any{"werf": "old"},
				Files:       map[string][]byte{},
				Metadata: werfdata.Metadata{
					APIVersion:       werfdata.APIVersionV2,
					Values:           werfdata.ValuesMetadata{Values: true, ExtraValues: true, ValuesSet: []string{"s=1", "a=two"}},
					ExtraAnnotations: map[string]string{"team": "billing", "tier": "backend"},
				},
				Values: map[string]any{"a": "one", "nested": map[string]any{"k": "v", "k2": "v2"}},
			},
		},
		{
			name:  "repack merges values files in order onto stored values",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{
				{
					inputs: map[string]string{
						"values1.yaml": "a:\n  x: one\n",
						"secret1.yaml": encryptChartPackTestValues(t, "a:\n  x: one\n"),
					},
					opts: ChartPackOptions{ValuesFiles: []string{"values1.yaml"}, SecretValuesFiles: []string{"secret1.yaml"}},
				},
				{
					inputs: map[string]string{
						"values2.yaml": "a: zero\n",
						"values3.yaml": "a:\n  z: two\n",
						"secret2.yaml": encryptChartPackTestValues(t, "a: zero\n"),
						"secret3.yaml": encryptChartPackTestValues(t, "a:\n  z: two\n"),
					},
					opts: ChartPackOptions{
						ValuesFiles:       []string{"values2.yaml", "values3.yaml"},
						SecretValuesFiles: []string{"secret2.yaml", "secret3.yaml"},
					},
				},
			},
			expectedData: &werfdata.Data{
				Files: map[string][]byte{},
				Metadata: werfdata.Metadata{
					APIVersion: werfdata.APIVersionV2,
					Values:     werfdata.ValuesMetadata{Values: true, SecretValues: true},
				},
				Values: map[string]any{"a": map[string]any{"z": "two"}},
			},
			expectedSecretValues: map[string]any{"a": map[string]any{"z": "two"}},
		},
		{
			name:  "repack replaces only the passed disable switches",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{
				{opts: ChartPackOptions{
					DefaultPatchesDisable:      lo.ToPtr(true),
					DefaultSecretValuesDisable: lo.ToPtr(true),
					DefaultValuesDisable:       lo.ToPtr(true),
				}},
				{opts: ChartPackOptions{DefaultValuesDisable: lo.ToPtr(false)}},
			},
			expectedData: &werfdata.Data{
				Files: map[string][]byte{},
				Metadata: werfdata.Metadata{
					APIVersion:            werfdata.APIVersionV2,
					Values:                werfdata.ValuesMetadata{DefaultSecretValuesDisable: true},
					DefaultPatchesDisable: true,
				},
			},
		},
		{
			name: "invalid .werf-data of a dependency rejected",
			chart: map[string]string{
				"Chart.yaml":                                        chartPackTestChartYAML,
				"charts/dep/Chart.yaml":                             "apiVersion: v2\nname: dep\nversion: 1.0.0\n",
				"charts/dep/charts/nested/Chart.yaml":               "apiVersion: v2\nname: nested\nversion: 1.0.0\n",
				"charts/dep/charts/nested/.werf-data/metadata.yaml": "apiVersion: v3\n",
			},
			steps:       []chartPackTestStep{{}},
			expectedErr: `dependency "dep": dependency "nested": unsupported .werf-data/metadata.yaml apiVersion "v3"`,
		},
		{
			name: "plaintext stored secret values rejected",
			chart: map[string]string{
				"Chart.yaml":                    chartPackTestChartYAML,
				".werf-data/metadata.yaml":      "apiVersion: v2\nvalues:\n  secretValues: true\n",
				".werf-data/secret-values.yaml": "password: plain\n",
			},
			steps:       []chartPackTestStep{{}},
			expectedErr: `.werf-data/secret-values.yaml: value at "password" is not encrypted`,
		},
		{
			name:  "first plaintext secret value in key order reported",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{
				inputs: map[string]string{"keys.yaml": "z: plain\ny: plain\nx: plain\nw: plain\nv: plain\nu: plain\nt: plain\ns: plain\nr: plain\nq: plain\np: plain\no: plain\nn: plain\nm: plain\nl: plain\nk: plain\nj: plain\ni: plain\nh: plain\ng: plain\nf: plain\ne: plain\nd: plain\nc: plain\nb: plain\na: plain\n"},
				opts:   ChartPackOptions{SecretValuesFiles: []string{"keys.yaml"}},
			}},
			expectedErr: `secret values file "keys.yaml": value at "a" is not encrypted`,
		},
		{
			name:  "secret values file with a leading and a trailing document separator",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{
				inputs: map[string]string{"values-a.yaml": "---\n" + encryptedPassword + "---\n"},
				opts:   ChartPackOptions{SecretValuesFiles: []string{"values-a.yaml"}},
			}},
			expectedData: &werfdata.Data{
				Files: map[string][]byte{},
				Metadata: werfdata.Metadata{
					APIVersion: werfdata.APIVersionV2,
					Values:     werfdata.ValuesMetadata{SecretValues: true},
				},
			},
			expectedSecretValues: map[string]any{"password": "p1", "list": []any{"s1", "s2"}},
		},
		{
			name: "stored secret values with consecutive document headers rejected",
			chart: map[string]string{
				"Chart.yaml":                    chartPackTestChartYAML,
				".werf-data/metadata.yaml":      "apiVersion: v2\nvalues:\n  secretValues: true\n",
				".werf-data/secret-values.yaml": "---\n---\npassword: plain\n",
			},
			steps:       []chartPackTestStep{{}},
			expectedErr: ".werf-data/secret-values.yaml: has several YAML documents",
		},
		{
			name:  "secret values file with consecutive document headers rejected",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{
				inputs: map[string]string{"values-a.yaml": "---\n---\npassword: plain\n"},
				opts:   ChartPackOptions{SecretValuesFiles: []string{"values-a.yaml"}},
			}},
			expectedErr: `secret values file "values-a.yaml": has several YAML documents`,
		},
		{
			name: "dependency secret values with consecutive document headers rejected",
			chart: map[string]string{
				"Chart.yaml":                               chartPackTestChartYAML,
				"charts/dep/Chart.yaml":                    "apiVersion: v2\nname: dep\nversion: 1.0.0\n",
				"charts/dep/.werf-data/metadata.yaml":      "apiVersion: v2\nvalues:\n  secretValues: true\n",
				"charts/dep/.werf-data/secret-values.yaml": "---\n---\npassword: plain\n",
			},
			steps:       []chartPackTestStep{{}},
			expectedErr: `dependency "dep": .werf-data/secret-values.yaml: has several YAML documents`,
		},
		{
			name: "multi-document stored secret values rejected",
			chart: map[string]string{
				"Chart.yaml":                    chartPackTestChartYAML,
				".werf-data/metadata.yaml":      "apiVersion: v2\nvalues:\n  secretValues: true\n",
				".werf-data/secret-values.yaml": "{}\n---\npassword: plain\n",
			},
			steps:       []chartPackTestStep{{}},
			expectedErr: ".werf-data/secret-values.yaml: has several YAML documents",
		},
		{
			name:  "multi-document secret values file rejected",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{
				inputs: map[string]string{"secret.yaml": encryptedPassword + "---\nplain: text\n"},
				opts:   ChartPackOptions{SecretValuesFiles: []string{"secret.yaml"}},
			}},
			expectedErr: `secret values file "secret.yaml": has several YAML documents`,
		},
		{
			name: "plaintext secret values of a dependency rejected",
			chart: map[string]string{
				"Chart.yaml":                                             chartPackTestChartYAML,
				"charts/dep/Chart.yaml":                                  "apiVersion: v2\nname: dep\nversion: 1.0.0\n",
				"charts/dep/charts/nested/Chart.yaml":                    "apiVersion: v2\nname: nested\nversion: 1.0.0\n",
				"charts/dep/charts/nested/.werf-data/metadata.yaml":      "apiVersion: v2\nvalues:\n  secretValues: true\n",
				"charts/dep/charts/nested/.werf-data/secret-values.yaml": "password: plain\n",
			},
			steps:       []chartPackTestStep{{opts: ChartPackOptions{ResetValues: true}}},
			expectedErr: `dependency "dep": dependency "nested": .werf-data/secret-values.yaml: value at "password" is not encrypted`,
		},
		{
			name:        "invalid root set JSON rejected",
			chart:       map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps:       []chartPackTestStep{{opts: ChartPackOptions{RootSetJSON: []string{"r={bad"}}}},
			expectedErr: `parse root set JSON value "r={bad"`,
		},
		{
			name: "author images annotation untouched when there is nothing to change",
			chart: map[string]string{"Chart.yaml": `name: mychart
version: 1.0.0
annotations:
  helm.sh/images: |
    - {name: author, image: "nginx:1"}
`},
			steps: []chartPackTestStep{{
				expectedChartYAML: `name: mychart
version: 1.0.0
annotations:
  helm.sh/images: |
    - {name: author, image: "nginx:1"}
`,
				opts: ChartPackOptions{Images: []chartyaml.Image{}},
			}},
		},
		{
			name:  "repack with reset values",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{
				{
					inputs: map[string]string{"values1.yaml": "a: one\n", "file1.txt": "file one"},
					opts: ChartPackOptions{
						DefaultValuesDisable: lo.ToPtr(true),
						ValuesFiles:          []string{"values1.yaml"},
						ValuesSet:            []string{"s=1"},
						ValuesSetFile:        []string{"f=file1.txt"},
						ExtraLabels:          map[string]string{"tier": "backend"},
						ExtraValues:          map[string]interface{}{"werf": "old"},
						Images:               []chartyaml.Image{{Name: "backend", Image: "registry/app:backend"}},
						Release:              &werfdata.ReleaseMetadata{Name: "myapp"},
					},
				},
				{
					expectedChartYAML: strings.Replace(chartPackTestChartYAML, "v2   #", "v2 #", 1),
					opts: ChartPackOptions{
						ValuesSet:   []string{"b=1"},
						ResetValues: true,
					},
				},
			},
			expectedData: &werfdata.Data{
				Files: map[string][]byte{},
				Metadata: werfdata.Metadata{
					APIVersion: werfdata.APIVersionV2,
					Values:     werfdata.ValuesMetadata{ValuesSet: []string{"b=1"}},
				},
			},
		},
		{
			name:  "same set-file path with different contents across packs",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{
				{
					inputs: map[string]string{"file1.txt": "first"},
					opts:   ChartPackOptions{ValuesSetFile: []string{"f=file1.txt"}},
				},
				{
					inputs: map[string]string{"file1.txt": "second"},
					opts:   ChartPackOptions{ValuesSetFile: []string{"f=file1.txt"}},
				},
			},
			expectedData: &werfdata.Data{
				Files: map[string][]byte{"set-files/0": []byte("first"), "set-files/1": []byte("second")},
				Metadata: werfdata.Metadata{
					APIVersion: werfdata.APIVersionV2,
					Values: werfdata.ValuesMetadata{ValuesSetFile: []werfdata.SetFileRecord{
						{Value: "f=file1.txt", Contents: map[string]string{"file1.txt": "set-files/0"}},
						{Value: "f=file1.txt", Contents: map[string]string{"file1.txt": "set-files/1"}},
					}},
				},
			},
		},
		{
			name: "images added, replaced and removed, author entry kept",
			chart: map[string]string{"Chart.yaml": `name: mychart
version: 1.0.0
annotations:
  helm.sh/images: |
    - name: author
      image: nginx:1
`},
			steps: []chartPackTestStep{
				{
					expectedChartYAML: `name: mychart
version: 1.0.0
annotations:
  helm.sh/images: |
    - name: author
      image: nginx:1
    - name: backend
      image: registry/app:backend
    - name: frontend
      image: registry/app:frontend
`,
					opts: ChartPackOptions{Images: []chartyaml.Image{
						{Name: "backend", Image: "registry/app:backend"},
						{Name: "frontend", Image: "registry/app:frontend"},
					}},
				},
				{
					expectedChartYAML: `name: mychart
version: 1.0.0
annotations:
  helm.sh/images: |
    - name: author
      image: nginx:1
    - name: frontend
      image: registry/app:frontend2
`,
					opts: ChartPackOptions{Images: []chartyaml.Image{{Name: "frontend", Image: "registry/app:frontend2"}}},
				},
				{
					expectedChartYAML: `name: mychart
version: 1.0.0
annotations:
  helm.sh/images: |
    - name: author
      image: nginx:1
`,
					opts: ChartPackOptions{Images: []chartyaml.Image{}},
				},
			},
			expectedData: &werfdata.Data{
				Files:    map[string][]byte{},
				Metadata: werfdata.Metadata{APIVersion: werfdata.APIVersionV2},
			},
		},
		{
			name: "image colliding with an author entry",
			chart: map[string]string{"Chart.yaml": `name: mychart
version: 1.0.0
annotations:
  helm.sh/images: |
    - name: author
      image: nginx:1
`},
			steps: []chartPackTestStep{
				{opts: ChartPackOptions{Images: []chartyaml.Image{{Name: "author", Image: "registry/app:author"}}}},
			},
			expectedErr: `helm.sh/images in Chart.yaml already has an entry "author"; remove it from Chart.yaml or rename the werf image`,
		},
		{
			name: "Chart.yaml and Chart.lock formatting survives",
			chart: map[string]string{
				"Chart.yaml": chartPackTestChartYAML + "description: >-\n  folded\n  text\nkeywords: [a, b]\n",
				"Chart.lock": "# lock comment\ndependencies: []\ndigest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n",
			},
			steps: []chartPackTestStep{
				{
					expectedChartYAML: `# The chart.
apiVersion: v2 # chart api
name: renamed
# Bumped by CI.
version: 2.0.0
annotations:
  team: payments # owner
description: >-
  folded
  text
keywords: [a, b]
appVersion: "1.2"
`,
					opts: ChartPackOptions{Name: "renamed", Version: "2.0.0", AppVersion: "1.2"},
				},
			},
			expectedFiles: []string{"renamed/Chart.lock:# lock comment\ndependencies: []\ndigest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n"},
		},
		{
			name: "v1 bundle extra annotations and labels migrated",
			chart: map[string]string{
				"Chart.yaml":             chartPackTestChartYAML,
				"extra_annotations.json": `{"team": "payments", "project.werf.io/name": "myapp", "ci.werf.io/commit": "abc", "werf.io/release-channel": "stable"}`,
				"extra_labels.json":      `{"tier": "backend"}`,
				"README.md":              "readme",
			},
			steps: []chartPackTestStep{{opts: ChartPackOptions{ExtraLabels: map[string]string{"tier": "frontend"}}}},
			expectedData: &werfdata.Data{
				Files: map[string][]byte{},
				Metadata: werfdata.Metadata{
					APIVersion:       werfdata.APIVersionV2,
					ExtraAnnotations: map[string]string{"team": "payments"},
					ExtraLabels:      map[string]string{"tier": "frontend"},
					ExtraRuntimeAnnotations: map[string]string{
						"project.werf.io/name":    "myapp",
						"ci.werf.io/commit":       "abc",
						"werf.io/release-channel": "stable",
					},
					ReleaseInfoAnnotations: map[string]string{
						"project.werf.io/name":    "myapp",
						"ci.werf.io/commit":       "abc",
						"werf.io/release-channel": "stable",
					},
				},
			},
			expectedFiles: []string{"mychart/README.md:readme", "!mychart/extra_annotations.json", "!mychart/extra_labels.json"},
		},
		{
			name:  "plaintext secret values rejected",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{
				inputs: map[string]string{"secret.yaml": "db:\n  password: plain\n"},
				opts:   ChartPackOptions{SecretValuesFiles: []string{"secret.yaml"}},
			}},
			expectedErr: `secret values file "secret.yaml": value at "db.password" is not encrypted`,
		},
		{
			name:  "oversized .werf-data file rejected",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{{
				inputs: map[string]string{"big.txt": strings.Repeat("x", 5*1024*1024+1)},
				opts:   ChartPackOptions{ValuesSetFile: []string{"big=big.txt"}},
			}},
			expectedErr: ".werf-data/set-files/0 is 5242881 bytes, more than the 5242880 bytes a chart loader accepts for a single file",
		},
		{
			name:  "JSON Chart.yaml of a werf 3.12 bundle",
			chart: map[string]string{"Chart.yaml": `{"apiVersion":"v2","name":"mychart","version":"1.0.0","annotations":{"team":"payments"}}`},
			steps: []chartPackTestStep{{
				expectedChartYAML: "apiVersion: v2\nname: mychart\nversion: 2.0.0\nannotations:\n  team: payments\n  helm.sh/images: |\n    - name: backend\n      image: registry/app:backend\n",
				opts: ChartPackOptions{
					Images:  []chartyaml.Image{{Name: "backend", Image: "registry/app:backend"}},
					Version: "2.0.0",
				},
			}},
			expectedData: &werfdata.Data{
				Files:    map[string][]byte{},
				Metadata: werfdata.Metadata{APIVersion: werfdata.APIVersionV2, Images: []string{"backend"}},
			},
		},
		{
			name: "vendored dependency directory of another locked version rejected",
			chart: map[string]string{
				"Chart.yaml":            "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.2.0\n    repository: https://example.invalid\n",
				"Chart.lock":            "dependencies:\n- name: dep\n  repository: https://example.invalid\n  version: 0.2.0\ndigest: sha256:abc\ngenerated: \"2024-01-01T00:00:00Z\"\n",
				"charts/dep/Chart.yaml": "apiVersion: v2\nname: dep\nversion: 0.1.0\n",
			},
			steps:       []chartPackTestStep{{}},
			expectedErr: "charts/dep is version 0.1.0 but Chart.lock requires 0.2.0: remove charts/dep or vendor version 0.2.0 there",
		},
		{
			name: "vendored dependency directory not matching the unlocked constraint rejected",
			chart: map[string]string{
				"Chart.yaml":            "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.2.x\n    repository: https://example.invalid\n",
				"charts/dep/Chart.yaml": "apiVersion: v2\nname: dep\nversion: 0.1.0\n",
			},
			steps:       []chartPackTestStep{{}},
			expectedErr: "charts/dep is version 0.1.0 but Chart.yaml requires 0.2.x: remove charts/dep or vendor a matching version there",
		},
		{
			name:  "v3 chart",
			chart: map[string]string{"Chart.yaml": "apiVersion: v3\nname: mychart\nversion: 1.0.0\n"},
			steps: []chartPackTestStep{{
				expectedChartYAML: "apiVersion: v3\nname: mychart\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: registry/app:backend\n",
				opts: ChartPackOptions{
					ValuesSet: []string{"s=1"},
					Images:    []chartyaml.Image{{Name: "backend", Image: "registry/app:backend"}},
				},
			}},
			expectedData: &werfdata.Data{
				Files: map[string][]byte{},
				Metadata: werfdata.Metadata{
					APIVersion: werfdata.APIVersionV2,
					Values:     werfdata.ValuesMetadata{ValuesSet: []string{"s=1"}},
					Images:     []string{"backend"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			t.Chdir(workDir)

			chartDir := filepath.Join(t.TempDir(), "mychart")
			writeChartPackTestFiles(t, chartDir, tt.chart)

			var (
				archivePath string
				err         error
			)

			for i, step := range tt.steps {
				writeChartPackTestFiles(t, workDir, step.inputs)

				opts := step.opts
				opts.Destination = t.TempDir()

				var result *ChartPackResult

				result, err = ChartPack(newChartPackTestContext(), chartDir, opts)
				if err != nil {
					require.Equal(t, len(tt.steps)-1, i, "unexpected error in step %d: %s", i, err)
					break
				}

				archivePath = result.ArchivePath

				if step.expectedChartYAML != "" {
					assert.Equal(t, step.expectedChartYAML, readChartPackTestArchiveFile(t, archivePath, result.ChartName+"/Chart.yaml"), "step %d", i)
				}

				unpackDir := t.TempDir()
				require.NoError(t, v2util.ExpandFile(unpackDir, archivePath))
				chartDir = filepath.Join(unpackDir, result.ChartName)
			}

			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)

				return
			}

			require.NoError(t, err)

			if tt.expectedData == nil {
				for name := range readChartPackTestArchiveFiles(t, archivePath) {
					_, chartFileName, _ := strings.Cut(name, "/")
					assert.False(t, werfdata.IsWerfDataFile(chartFileName), name)
				}

				return
			}

			data := loadChartPackTestWerfData(t, archivePath)

			if tt.expectedSecretValues != nil {
				assert.Equal(t, tt.expectedSecretValues, decryptChartPackTestValues(t, data.SecretValues))
			} else {
				assert.Nil(t, data.SecretValues)
			}

			data.SecretValues = nil
			assert.Equal(t, tt.expectedData, data)

			for _, expectedFile := range tt.expectedFiles {
				if name, ok := strings.CutPrefix(expectedFile, "!"); ok {
					assert.NotContains(t, readChartPackTestArchiveFiles(t, archivePath), name)
					continue
				}

				name, contents, _ := strings.Cut(expectedFile, ":")
				assert.Equal(t, contents, readChartPackTestArchiveFile(t, archivePath, name))
			}
		})
	}
}

func TestChartPack_ChecksDecompressedChartSize(t *testing.T) {
	maxChartSize := archive.MaxDecompressedChartSize
	archive.MaxDecompressedChartSize = 4096

	t.Cleanup(func() {
		archive.MaxDecompressedChartSize = maxChartSize
	})

	t.Chdir(t.TempDir())
	writeChartPackTestFiles(t, ".", map[string]string{"file.txt": strings.Repeat("x", 2000)})

	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{
		"Chart.yaml":                   "apiVersion: v2\nname: mychart\nversion: 1.0.0\n",
		"charts/dep/Chart.yaml":        "apiVersion: v2\nname: dep\nversion: 1.0.0\n",
		"charts/dep/templates/cm.yaml": "# " + strings.Repeat("x", 2000) + "\n",
	})

	destination := t.TempDir()
	_, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination:   destination,
		ValuesSetFile: []string{"f=file.txt"},
	})
	require.ErrorContains(t, err, "a chart loader accepts less than 4096 bytes")
	assert.NoFileExists(t, filepath.Join(destination, "mychart-1.0.0.tgz"))
}

func TestChartPack_FailedArchiveMoveRemovesOldProvenance(t *testing.T) {
	keyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.secret")
	require.NoError(t, err)

	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{"Chart.yaml": "apiVersion: v2\nname: mychart\nversion: 1.0.0\n"})

	// A non-empty directory in place of the archive makes moving the archive into place fail.
	destination := t.TempDir()
	writeChartPackTestFiles(t, destination, map[string]string{
		"mychart-1.0.0.tgz/keep": "",
		"mychart-1.0.0.tgz.prov": "old signature",
	})

	_, err = ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination: destination,
		Sign:        true,
		SignKey:     "helm-test",
		SignKeyring: keyringPath,
	})
	require.ErrorContains(t, err, "move archive into place")
	assert.NoFileExists(t, filepath.Join(destination, "mychart-1.0.0.tgz.prov"))
}

func TestChartPack_FailedPackLeavesDestinationUntouched(t *testing.T) {
	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{"Chart.yaml": "apiVersion: v2\nname: mychart\nversion: 1.0.0\n"})

	// Nothing, not even a staging directory, may appear in the chart while packing it into a
	// destination inside it, since a killed pack would leave it for the next pack to pick up.
	_, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination: filepath.Join(chartDir, "out"),
		Sign:        true,
		SignKey:     "helm-test",
		SignKeyring: filepath.Join(t.TempDir(), "missing.secret"),
	})
	require.Error(t, err)

	entries, err := os.ReadDir(chartDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"Chart.yaml"}, lo.Map(entries, func(entry os.DirEntry, _ int) string {
		return entry.Name()
	}))
}

func TestChartPack_FailedResignKeepsSignedArchive(t *testing.T) {
	keyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.secret")
	require.NoError(t, err)
	publicKeyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.pub")
	require.NoError(t, err)

	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{"Chart.yaml": "apiVersion: v2\nname: mychart\nversion: 1.0.0\n"})

	destination := filepath.Join(t.TempDir(), "not", "created")
	signed, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination: destination,
		Sign:        true,
		SignKey:     "helm-test",
		SignKeyring: keyringPath,
	})
	require.NoError(t, err)

	_, err = ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination: destination,
		Sign:        true,
		SignKey:     "helm-test",
		SignKeyring: filepath.Join(t.TempDir(), "missing.secret"),
		ValuesSet:   []string{"a=1"},
	})
	require.Error(t, err)

	entries, err := os.ReadDir(destination)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"mychart-1.0.0.tgz", "mychart-1.0.0.tgz.prov"}, lo.Map(entries, func(entry os.DirEntry, _ int) string {
		return entry.Name()
	}))

	archiveData, err := os.ReadFile(signed.ArchivePath)
	require.NoError(t, err)
	provData, err := os.ReadFile(signed.ProvenancePath)
	require.NoError(t, err)

	signatory, err := provenance.NewFromKeyring(publicKeyringPath, "")
	require.NoError(t, err)
	_, err = signatory.Verify(archiveData, provData, filepath.Base(signed.ArchivePath))
	require.NoError(t, err)
}

func TestChartPack_ForwardsRepositorySettings(t *testing.T) {
	setupChartPackTestDependencyCaches(t)
	server := newChartPackTestChartRepo(t)

	repositoryConfig := filepath.Join(t.TempDir(), "repositories.yaml")
	writeChartPackTestFiles(t, filepath.Dir(repositoryConfig), map[string]string{
		"repositories.yaml": fmt.Sprintf("apiVersion: v1\nrepositories:\n- name: myrepo\n  url: %s\n", server.URL),
	})

	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.1.0\n    repository: \"@myrepo\"\n",
	})

	repositoryCache := t.TempDir()
	result, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		ContentCache:     t.TempDir(),
		DependencyUpdate: true,
		Destination:      t.TempDir(),
		RepositoryCache:  repositoryCache,
		RepositoryConfig: repositoryConfig,
	})
	require.NoError(t, err)

	assert.Contains(t, readChartPackTestArchiveFiles(t, result.ArchivePath), "mychart/charts/dep/templates/cm.yaml")
	assert.FileExists(t, filepath.Join(repositoryCache, "myrepo-index.yaml"))
}

func TestChartPack_RelativeDestination(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)

	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{"Chart.yaml": "apiVersion: v2\nname: mychart\nversion: 1.0.0\n"})

	result, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{Destination: "."})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workDir, "mychart-1.0.0.tgz"), result.ArchivePath)
	assert.FileExists(t, result.ArchivePath)
}

func TestChartPack_RemovesStaleProvenance(t *testing.T) {
	keyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.secret")
	require.NoError(t, err)

	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{"Chart.yaml": "apiVersion: v2\nname: mychart\nversion: 1.0.0\n"})

	destination := t.TempDir()
	signed, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination: destination,
		Sign:        true,
		SignKey:     "helm-test",
		SignKeyring: keyringPath,
	})
	require.NoError(t, err)
	require.FileExists(t, signed.ProvenancePath)

	unsigned, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination: destination,
		ValuesSet:   []string{"a=1"},
	})
	require.NoError(t, err)
	assert.Empty(t, unsigned.ProvenancePath)
	assert.NoFileExists(t, signed.ProvenancePath)
}

func TestChartPack_RepackKeepsVendoredDependencies(t *testing.T) {
	setupChartPackTestDependencyCaches(t)
	server := newChartPackTestChartRepo(t)

	sourceDir := t.TempDir()
	writeChartPackTestFiles(t, filepath.Join(sourceDir, "local"), map[string]string{
		"Chart.yaml":        "apiVersion: v2\nname: local\nversion: 0.2.0\n",
		"templates/cm.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: local\n",
	})

	deps := []*v2chart.Dependency{
		{Name: "dep", Version: "0.1.0", Repository: server.URL},
		{Name: "local", Version: "0.2.0", Repository: "file://../local"},
	}
	digest, err := resolver.HashReq(deps, deps)
	require.NoError(t, err)

	writeChartPackTestFiles(t, filepath.Join(sourceDir, "mychart"), map[string]string{
		"Chart.yaml": fmt.Sprintf("apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.1.0\n    repository: %s\n  - name: local\n    version: 0.2.0\n    repository: file://../local\n", server.URL),
		"Chart.lock": fmt.Sprintf("dependencies:\n- name: dep\n  repository: %s\n  version: 0.1.0\n- name: local\n  repository: file://../local\n  version: 0.2.0\ndigest: %s\ngenerated: \"2024-01-01T00:00:00Z\"\n", server.URL, digest),
	})

	packed, err := ChartPack(newChartPackTestContext(), filepath.Join(sourceDir, "mychart"), ChartPackOptions{Destination: t.TempDir()})
	require.NoError(t, err)

	// The file:// source stays next to the unpacked chart, the chart repository goes away.
	server.Close()
	require.NoError(t, os.RemoveAll(filepath.Join(sourceDir, "mychart")))
	require.NoError(t, v2util.ExpandFile(sourceDir, packed.ArchivePath))

	repacked, err := ChartPack(newChartPackTestContext(), filepath.Join(sourceDir, "mychart"), ChartPackOptions{
		Destination: t.TempDir(),
		ValuesSet:   []string{"a=1"},
	})
	require.NoError(t, err)

	files := readChartPackTestArchiveFiles(t, repacked.ArchivePath)
	assert.Contains(t, files, "mychart/charts/dep/templates/cm.yaml")
	assert.Contains(t, files, "mychart/charts/local/templates/cm.yaml")
	assert.Equal(t, []string{"a=1"}, loadChartPackTestWerfData(t, repacked.ArchivePath).Metadata.Values.ValuesSet)
}

func TestChartPack_SecretValuesRejectedWithoutSource(t *testing.T) {
	contents := []struct {
		name        string
		data        string
		expectedErr string
	}{
		{name: "malformed", data: "password: TOPSECRET: invalid\n", expectedErr: "invalid YAML"},
		{name: "duplicate key", data: "password: TOPSECRET\npassword: \"\"\n", expectedErr: `duplicate key "password" at line 2, column 1`},
		{name: "nested duplicate key", data: "db:\n  password: TOPSECRET\n  password: \"\"\n", expectedErr: `duplicate key "password" at line 3, column 3`},
		{name: "not a mapping", data: "- TOPSECRET\n", expectedErr: "expected a mapping at the top level, got a list at line 1"},
	}

	locations := []struct {
		name      string
		chart     func(data string) map[string]string
		opts      ChartPackOptions
		errPrefix string
	}{
		{
			name:      "secret values file",
			chart:     func(string) map[string]string { return map[string]string{"Chart.yaml": chartPackTestChartYAML} },
			opts:      ChartPackOptions{SecretValuesFiles: []string{"values-a.yaml"}},
			errPrefix: `secret values file "values-a.yaml": `,
		},
		{
			name: "stored secret values",
			chart: func(data string) map[string]string {
				return map[string]string{
					"Chart.yaml":                    chartPackTestChartYAML,
					".werf-data/metadata.yaml":      "apiVersion: v2\nvalues:\n  secretValues: true\n",
					".werf-data/secret-values.yaml": data,
				}
			},
			errPrefix: ".werf-data/secret-values.yaml: ",
		},
		{
			name: "dependency secret values",
			chart: func(data string) map[string]string {
				return map[string]string{
					"Chart.yaml":                               chartPackTestChartYAML,
					"charts/dep/Chart.yaml":                    "apiVersion: v2\nname: dep\nversion: 1.0.0\n",
					"charts/dep/.werf-data/metadata.yaml":      "apiVersion: v2\nvalues:\n  secretValues: true\n",
					"charts/dep/.werf-data/secret-values.yaml": data,
				}
			},
			errPrefix: `dependency "dep": .werf-data/secret-values.yaml: `,
		},
	}

	for _, content := range contents {
		for _, location := range locations {
			t.Run(content.name+" in "+location.name, func(t *testing.T) {
				workDir := t.TempDir()
				t.Chdir(workDir)
				writeChartPackTestFiles(t, workDir, map[string]string{"values-a.yaml": content.data})

				chartDir := filepath.Join(t.TempDir(), "mychart")
				writeChartPackTestFiles(t, chartDir, location.chart(content.data))

				opts := location.opts
				opts.Destination = t.TempDir()

				_, err := ChartPack(newChartPackTestContext(), chartDir, opts)
				require.ErrorContains(t, err, location.errPrefix+content.expectedErr)
				assert.NotContains(t, err.Error(), "TOPSECRET")
			})
		}
	}
}

func TestChartPack_Sign(t *testing.T) {
	keyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.secret")
	require.NoError(t, err)
	publicKeyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.pub")
	require.NoError(t, err)

	for _, apiVersion := range []string{"v2", "v3"} {
		t.Run(apiVersion, func(t *testing.T) {
			chartDir := filepath.Join(t.TempDir(), "mychart")
			writeChartPackTestFiles(t, chartDir, map[string]string{"Chart.yaml": "apiVersion: " + apiVersion + "\nname: mychart\nversion: 1.0.0\n"})

			result, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
				ValuesSet:   []string{"a=1"},
				Destination: t.TempDir(),
				Sign:        true,
				SignKey:     "helm-test",
				SignKeyring: keyringPath,
			})
			require.NoError(t, err)
			require.Equal(t, result.ArchivePath+".prov", result.ProvenancePath)

			archiveData, err := os.ReadFile(result.ArchivePath)
			require.NoError(t, err)
			provData, err := os.ReadFile(result.ProvenancePath)
			require.NoError(t, err)

			signatory, err := provenance.NewFromKeyring(publicKeyringPath, "")
			require.NoError(t, err)
			_, err = signatory.Verify(archiveData, provData, filepath.Base(result.ArchivePath))
			require.NoError(t, err)
		})
	}
}

func TestChartPack_StagesInTempDir(t *testing.T) {
	chartDir := filepath.Join(t.TempDir(), "mychart")
	writeChartPackTestFiles(t, chartDir, map[string]string{"Chart.yaml": "apiVersion: v2\nname: mychart\nversion: 1.0.0\n"})

	_, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
		Destination: t.TempDir(),
		TempDirPath: filepath.Join(t.TempDir(), "missing"),
	})
	require.ErrorContains(t, err, "create staging directory")
}

func TestChartPack_TypeScriptBundle(t *testing.T) {
	t.Setenv(featgate.FeatGateTypescript.EnvVarName(), "true")

	for _, apiVersion := range []string{"v2", "v3"} {
		t.Run(apiVersion, func(t *testing.T) {
			chartDir := filepath.Join(t.TempDir(), "mychart")
			writeChartPackTestFiles(t, chartDir, map[string]string{
				"Chart.yaml":             "apiVersion: " + apiVersion + "\nname: mychart\nversion: 1.0.0\n",
				"ts/src/index.ts":        "export {};\n",
				common.ChartTSBundleFile: "bundle\n",
			})

			result, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
				DenoBinaryPath: filepath.Join(t.TempDir(), "deno"),
				Destination:    t.TempDir(),
				TSNoRebuild:    true,
			})
			require.NoError(t, err)

			files := readChartPackTestArchiveFiles(t, result.ArchivePath)
			assert.Equal(t, "bundle\n", files["mychart/"+common.ChartTSBundleFile])
			assert.Equal(t, "export {};\n", files["mychart/ts/src/index.ts"])
		})
	}
}

func TestChartPack_VendorsDependenciesFromChartLock(t *testing.T) {
	for _, skipRepoUpdate := range []bool{false, true} {
		t.Run(fmt.Sprintf("skip repo update %t", skipRepoUpdate), func(t *testing.T) {
			setupChartPackTestDependencyCaches(t)
			server := newChartPackTestChartRepo(t)

			deps := []*v2chart.Dependency{{Name: "dep", Version: "0.1.0", Repository: server.URL}}
			digest, err := resolver.HashReq(deps, deps)
			require.NoError(t, err)

			chartLock := fmt.Sprintf("dependencies:\n- name: dep\n  repository: %s\n  version: 0.1.0\ndigest: %s\ngenerated: \"2024-01-01T00:00:00Z\"\n", server.URL, digest)
			chartDir := filepath.Join(t.TempDir(), "mychart")
			writeChartPackTestFiles(t, chartDir, map[string]string{
				"Chart.yaml": fmt.Sprintf("apiVersion: v2\nname: mychart\nversion: 1.0.0\ndependencies:\n  - name: dep\n    version: 0.1.0\n    repository: %s\n", server.URL),
				"Chart.lock": chartLock,
			})

			result, err := ChartPack(newChartPackTestContext(), chartDir, ChartPackOptions{
				ChartRepoSkipUpdate: skipRepoUpdate,
				Destination:         t.TempDir(),
			})
			require.NoError(t, err)

			files := readChartPackTestArchiveFiles(t, result.ArchivePath)
			assert.Equal(t, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: dep\n", files["mychart/charts/dep/templates/cm.yaml"])
			assert.Equal(t, chartLock, files["mychart/Chart.lock"])
		})
	}
}

// newChartPackTestChartRepo serves a chart repository with the dep chart 0.1.0.
func newChartPackTestChartRepo(t *testing.T) *httptest.Server {
	t.Helper()

	repoDir := t.TempDir()
	server := httptest.NewServer(http.FileServer(http.Dir(repoDir)))
	t.Cleanup(server.Close)

	depDir := filepath.Join(t.TempDir(), "dep")
	writeChartPackTestFiles(t, depDir, map[string]string{
		"Chart.yaml":        "apiVersion: v2\nname: dep\nversion: 0.1.0\n",
		"templates/cm.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: dep\n",
	})

	depChart, err := v2loader.Load(context.Background(), depDir)
	require.NoError(t, err)
	_, err = v2util.Save(depChart, repoDir)
	require.NoError(t, err)

	index, err := repo.IndexDirectory(repoDir, server.URL)
	require.NoError(t, err)
	require.NoError(t, index.WriteFile(filepath.Join(repoDir, "index.yaml"), 0o644))

	return server
}

func readChartPackTestArchiveFile(t *testing.T, archivePath, name string) string {
	t.Helper()

	files := readChartPackTestArchiveFiles(t, archivePath)
	require.Contains(t, files, name)

	return files[name]
}

func decryptChartPackTestValues(t *testing.T, encrypted []byte) map[string]any {
	t.Helper()

	encoder, err := secret.NewAesEncoder([]byte(secretValuesTypesSecretKey))
	require.NoError(t, err)

	decrypted, err := secret.NewYamlEncoder(encoder).DecryptYamlData(encrypted)
	require.NoError(t, err)

	vals := map[string]any{}
	require.NoError(t, yaml.Unmarshal(decrypted, &vals))

	return vals
}

func encryptChartPackTestValues(t *testing.T, plaintext string) string {
	t.Helper()

	encoder, err := secret.NewAesEncoder([]byte(secretValuesTypesSecretKey))
	require.NoError(t, err)

	encrypted, err := secret.NewYamlEncoder(encoder).EncryptYamlData([]byte(plaintext))
	require.NoError(t, err)

	return string(encrypted)
}

func loadChartPackTestWerfData(t *testing.T, archivePath string) *werfdata.Data {
	t.Helper()

	loaded, err := loader.LoadFile(context.Background(), archivePath)
	require.NoError(t, err)

	accessor, err := helmchart.NewAccessor(loaded)
	require.NoError(t, err)

	data, err := werfdata.Parse(accessor.WerfDataFiles())
	require.NoError(t, err)

	return data
}

func newChartPackTestContext() context.Context {
	return logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
}

func readChartPackTestArchiveFiles(t *testing.T, archivePath string) map[string]string {
	t.Helper()

	archiveFile, err := os.Open(archivePath)
	require.NoError(t, err)

	defer archiveFile.Close()

	gzipReader, err := gzip.NewReader(archiveFile)
	require.NoError(t, err)

	files := map[string]string{}
	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)

		data, err := io.ReadAll(tarReader)
		require.NoError(t, err)

		require.NotContains(t, files, header.Name, "duplicate archive entry")
		files[header.Name] = string(data)
	}

	return files
}

func setupChartPackTestDependencyCaches(t *testing.T) {
	t.Helper()

	t.Setenv("HELM_CACHE_HOME", t.TempDir())
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())
	t.Setenv("HELM_DATA_HOME", t.TempDir())

	localCacheDir, err := loader.LocalCacheDir()
	require.NoError(t, err)
	serviceDir, err := loader.ServiceDir()
	require.NoError(t, err)

	loader.SetLocalCacheDir(t.TempDir())
	loader.SetServiceDir(t.TempDir())

	t.Cleanup(func() {
		loader.SetLocalCacheDir(localCacheDir)
		loader.SetServiceDir(serviceDir)
	})
}

func writeChartPackTestFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, data := range files {
		filePath := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
		require.NoError(t, os.WriteFile(filePath, []byte(data), 0o644))
	}
}
