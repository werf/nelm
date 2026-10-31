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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/secret"
	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/chartyaml"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/intern/resolver"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
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
					ValuesOptions: common.ValuesOptions{
						DefaultValuesDisable: true,
						RootSetJSON:          []string{`r={"y":2}`},
						ValuesFiles:          []string{"values1.yaml", "values2.yaml"},
						ValuesSet:            []string{"s=1"},
						ValuesSetFile:        []string{"f=file1.txt,g=file2.txt"},
						ValuesSetJSON:        []string{`j={"x":1}`},
						ValuesSetLiteral:     []string{"l=3"},
						ValuesSetString:      []string{"ss=2"},
					},
					DefaultPatchesDisable:      true,
					DefaultSecretValuesDisable: true,
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
						ValuesOptions:    common.ValuesOptions{ValuesFiles: []string{"values1.yaml"}, ValuesSet: []string{"s=1"}},
						ExtraAnnotations: map[string]string{"team": "payments", "tier": "backend"},
						ExtraValues:      map[string]interface{}{"werf": "old"},
					},
				},
				{
					inputs: map[string]string{"values2.yaml": "nested:\n  k2: v2\n"},
					opts: ChartPackOptions{
						ValuesOptions:    common.ValuesOptions{ValuesFiles: []string{"values2.yaml"}, ValuesSet: []string{"a=two"}},
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
			name:  "repack with reset values",
			chart: map[string]string{"Chart.yaml": chartPackTestChartYAML},
			steps: []chartPackTestStep{
				{
					inputs: map[string]string{"values1.yaml": "a: one\n", "file1.txt": "file one"},
					opts: ChartPackOptions{
						ValuesOptions: common.ValuesOptions{
							DefaultValuesDisable: true,
							ValuesFiles:          []string{"values1.yaml"},
							ValuesSet:            []string{"s=1"},
							ValuesSetFile:        []string{"f=file1.txt"},
						},
						ExtraLabels: map[string]string{"tier": "backend"},
						ExtraValues: map[string]interface{}{"werf": "old"},
						Images:      []chartyaml.Image{{Name: "backend", Image: "registry/app:backend"}},
						Release:     &werfdata.ReleaseMetadata{Name: "myapp"},
					},
				},
				{
					expectedChartYAML: strings.Replace(chartPackTestChartYAML, "v2   #", "v2 #", 1),
					opts: ChartPackOptions{
						ValuesOptions: common.ValuesOptions{ValuesSet: []string{"b=1"}},
						ResetValues:   true,
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
					opts:   ChartPackOptions{ValuesOptions: common.ValuesOptions{ValuesSetFile: []string{"f=file1.txt"}}},
				},
				{
					inputs: map[string]string{"file1.txt": "second"},
					opts:   ChartPackOptions{ValuesOptions: common.ValuesOptions{ValuesSetFile: []string{"f=file1.txt"}}},
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
				opts:   ChartPackOptions{ValuesOptions: common.ValuesOptions{ValuesSetFile: []string{"big=big.txt"}}},
			}},
			expectedErr: ".werf-data/set-files/0 is 5242881 bytes, more than the 5242880 bytes a chart loader accepts for a single file",
		},
		{
			name:  "v3 chart",
			chart: map[string]string{"Chart.yaml": "apiVersion: v3\nname: mychart\nversion: 1.0.0\n"},
			steps: []chartPackTestStep{{
				expectedChartYAML: "apiVersion: v3\nname: mychart\nversion: 1.0.0\nannotations:\n  helm.sh/images: |\n    - name: backend\n      image: registry/app:backend\n",
				opts: ChartPackOptions{
					ValuesOptions: common.ValuesOptions{ValuesSet: []string{"s=1"}},
					Images:        []chartyaml.Image{{Name: "backend", Image: "registry/app:backend"}},
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
				ValuesOptions: common.ValuesOptions{ValuesSet: []string{"a=1"}},
				Destination:   t.TempDir(),
				Sign:          true,
				SignKey:       "helm-test",
				SignKeyring:   keyringPath,
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

func TestChartPack_VendorsDependenciesFromChartLock(t *testing.T) {
	for _, skipRepoUpdate := range []bool{false, true} {
		t.Run(fmt.Sprintf("skip repo update %t", skipRepoUpdate), func(t *testing.T) {
			setupChartPackTestDependencyCaches(t)

			repoDir := t.TempDir()

			server := httptest.NewServer(http.FileServer(http.Dir(repoDir)))
			defer server.Close()

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
