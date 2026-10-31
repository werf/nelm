package action

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	distributionregistry "github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/scheme"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmaction "github.com/werf/nelm/v2/pkg/helm/pkg/action"
	"github.com/werf/nelm/v2/pkg/helm/pkg/provenance"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/kube"
	"github.com/werf/nelm/v2/pkg/legacy/progrep"
	"github.com/werf/nelm/v2/pkg/release"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

const secretValuesTypesSecretKey = "0123456789abcdef0123456789abcdef"

type latestReleaseListLoads struct {
	errs      []error
	revisions []release.Revision
}

type latestReleaseListStorager struct {
	release.ReleaseStorager

	err              error
	historyLimit     int
	historyName      string
	historySummaries []release.RevisionSummary
	latestSummaries  []release.RevisionSummary
	loadGate         chan struct{}
	loadStarted      chan struct{}
	loads            *kdutil.Concurrent[*latestReleaseListLoads]
	revisions        []release.Revision
	selector         string
	skipAnnotations  bool
	summaries        map[string]*release.ReleaseSummary
}

func (s *latestReleaseListStorager) LatestRevisions(ctx context.Context, opts release.LatestRevisionsOptions) ([]release.Revision, error) {
	s.selector = opts.LabelSelector

	s.skipAnnotations = opts.SkipAnnotations
	if s.err != nil {
		return nil, s.err
	}

	revisions := slices.Clone(s.revisions)
	if opts.SkipAnnotations {
		for i := range revisions {
			revisions[i].Annotations = nil
		}
	}

	return revisions, nil
}

func (s *latestReleaseListStorager) ListLatestSummaries(ctx context.Context, opts release.ListLatestSummariesOptions) ([]release.RevisionSummary, error) {
	s.selector = opts.LabelSelector
	if s.err != nil {
		return nil, s.err
	}

	return s.latestSummaries, nil
}

func (s *latestReleaseListStorager) ListRevisionSummaries(ctx context.Context, name string, opts release.ListRevisionSummariesOptions) ([]release.RevisionSummary, error) {
	s.historyName, s.historyLimit = name, opts.Limit
	if s.err != nil {
		return nil, s.err
	}

	return s.historySummaries, nil
}

func (s *latestReleaseListStorager) LoadRevisionSummary(ctx context.Context, revision release.Revision) (*release.ReleaseSummary, error) {
	if s.loadGate != nil {
		s.loadStarted <- struct{}{}

		select {
		case <-s.loadGate:
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for load gate: %w", ctx.Err())
		}
	}

	var err error

	s.loads.RWTransaction(func(loads *latestReleaseListLoads) {
		loads.revisions = append(loads.revisions, revision)
		if len(loads.errs) > 0 {
			err = loads.errs[0]
			loads.errs = loads.errs[1:]
		}
	})

	if err != nil {
		return nil, err
	}

	return s.summaries[revision.Namespace+"/"+revision.Name], nil
}

func (s *latestReleaseListStorager) loadedRevisions() []release.Revision {
	if s.loads == nil {
		return nil
	}

	var revisions []release.Revision

	s.loads.RTransaction(func(loads *latestReleaseListLoads) {
		revisions = append(revisions, loads.revisions...)
	})

	return revisions
}

type progressPhasesAPIState struct {
	allowApply bool
	// appliedConfigMaps are the ConfigMaps applied by server-side apply, in order, if allowApply.
	appliedConfigMaps     []corev1.ConfigMap
	configMaps            map[string]corev1.ConfigMap
	deleteNamespaceStatus int
	// failConfigMaps are the names of the ConfigMaps whose next update or apply is forbidden.
	failConfigMaps       map[string]bool
	namespaceDeleted     bool
	requests             []string
	retainNamespace      bool
	trackNamespaceStatus int
}

type progressPhasesAPI struct {
	kubeOpts  common.KubeConnectionOptions
	onRequest func(*http.Request)
	state     *kdutil.Concurrent[*progressPhasesAPIState]
}

func newProgressPhasesAPI(t *testing.T) *progressPhasesAPI {
	t.Helper()

	api := &progressPhasesAPI{state: kdutil.NewConcurrent(&progressPhasesAPIState{
		configMaps: make(map[string]corev1.ConfigMap),
	})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if api.onRequest != nil {
			api.onRequest(r)
		}

		w.Header().Set("Content-Type", "application/json")
		api.state.RWTransaction(func(s *progressPhasesAPIState) {
			s.requests = append(s.requests, r.Method+" "+r.URL.Path)

			var response any

			status := http.StatusOK
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/version":
				response = version.Info{Major: "1", Minor: "33", GitVersion: "v1.33.0"}
			case r.Method == http.MethodGet && r.URL.Path == "/api":
				response = metav1.APIVersions{TypeMeta: metav1.TypeMeta{Kind: "APIVersions", APIVersion: "v1"}, Versions: []string{"v1"}}
			case r.Method == http.MethodGet && r.URL.Path == "/apis":
				response = metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"}, Groups: []metav1.APIGroup{}}
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1":
				response = metav1.APIResourceList{
					TypeMeta: metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"}, GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{Name: "namespaces", SingularName: "namespace", Kind: "Namespace", Verbs: metav1.Verbs{"get", "list", "delete"}},
						{Name: "configmaps", SingularName: "configmap", Kind: "ConfigMap", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create", "update", "delete"}},
					},
				}
			case r.URL.Path == "/api/v1/namespaces/progress-ns":
				switch r.Method {
				case http.MethodGet:
					if s.namespaceDeleted && s.trackNamespaceStatus != 0 {
						status = s.trackNamespaceStatus
					} else if s.namespaceDeleted && !s.retainNamespace {
						status = http.StatusNotFound
					} else {
						response = corev1.Namespace{TypeMeta: metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"}, ObjectMeta: metav1.ObjectMeta{Name: "progress-ns"}}
					}
				case http.MethodDelete:
					if s.deleteNamespaceStatus != 0 {
						status = s.deleteNamespaceStatus
					} else {
						s.namespaceDeleted = true
						response = metav1.Status{TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}, Status: metav1.StatusSuccess}
					}
				default:
					t.Errorf("unexpected namespace request: %s", r.Method)

					status = http.StatusNotFound
				}
			case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/progress-ns/configmaps"):
				key := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/progress-ns/configmaps")

				key = strings.TrimPrefix(key, "/")
				switch r.Method {
				case http.MethodGet:
					if key == "" {
						selector, err := labels.Parse(r.URL.Query().Get("labelSelector"))
						if err != nil {
							t.Errorf("parse label selector: %s", err)

							status = http.StatusBadRequest

							break
						}

						if strings.Contains(r.Header.Get("Accept"), "as=PartialObjectMetadataList") {
							list := metav1.PartialObjectMetadataList{TypeMeta: metav1.TypeMeta{Kind: "PartialObjectMetadataList", APIVersion: "meta.k8s.io/v1"}, Items: []metav1.PartialObjectMetadata{}}
							for _, cm := range s.configMaps {
								if selector.Matches(labels.Set(cm.Labels)) {
									list.Items = append(list.Items, metav1.PartialObjectMetadata{ObjectMeta: cm.ObjectMeta})
								}
							}

							response = list
						} else {
							list := corev1.ConfigMapList{TypeMeta: metav1.TypeMeta{Kind: "ConfigMapList", APIVersion: "v1"}, Items: []corev1.ConfigMap{}}
							for _, cm := range s.configMaps {
								if selector.Matches(labels.Set(cm.Labels)) {
									list.Items = append(list.Items, cm)
								}
							}

							response = list
						}
					} else if cm, found := s.configMaps[key]; found {
						response = cm
					} else {
						status = http.StatusNotFound
					}
				case http.MethodPost, http.MethodPut:
					var cm corev1.ConfigMap

					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read ConfigMap body: %s", err)

						status = http.StatusBadRequest

						break
					}

					if _, _, err := scheme.Codecs.UniversalDeserializer().Decode(body, nil, &cm); err != nil {
						t.Errorf("decode ConfigMap body: %s", err)

						status = http.StatusBadRequest

						break
					}

					cm.Kind, cm.APIVersion = "ConfigMap", "v1"
					cm.Namespace, cm.ResourceVersion = "progress-ns", "1"

					if r.Method == http.MethodPut && s.failConfigMaps[cm.Name] {
						delete(s.failConfigMaps, cm.Name)

						status = http.StatusForbidden

						break
					}

					s.configMaps[cm.Name] = cm
					response = cm
				case http.MethodPatch:
					dryRun := r.URL.Query().Get("dryRun") == "All"
					if !s.allowApply {
						assert.True(t, dryRun, "unexpected apply of ConfigMap %q", key)
					}

					var cm corev1.ConfigMap
					if err := json.NewDecoder(r.Body).Decode(&cm); err != nil {
						t.Errorf("decode applied ConfigMap: %s", err)

						status = http.StatusBadRequest

						break
					}

					cm.Namespace = "progress-ns"

					if !dryRun {
						if s.failConfigMaps[key] {
							delete(s.failConfigMaps, key)

							status = http.StatusForbidden

							break
						}

						cm.Kind, cm.APIVersion, cm.ResourceVersion = "ConfigMap", "v1", "1"
						s.configMaps[key] = cm
						s.appliedConfigMaps = append(s.appliedConfigMaps, cm)
					}

					response = cm
				case http.MethodDelete:
					delete(s.configMaps, key)

					response = metav1.Status{TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}, Status: metav1.StatusSuccess}
				default:
					t.Errorf("unexpected configmap request: %s %s", r.Method, r.URL)

					status = http.StatusNotFound
				}
			default:
				t.Errorf("unexpected Kubernetes request: %s %s", r.Method, r.URL)

				status = http.StatusNotFound
			}

			if status != http.StatusOK {
				reason := metav1.StatusReasonForbidden
				if status == http.StatusNotFound {
					reason = metav1.StatusReasonNotFound
				}

				response = metav1.Status{TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}, Status: metav1.StatusFailure, Reason: reason, Message: http.StatusText(status)}
			}

			w.WriteHeader(status)

			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("encode Kubernetes response: %s", err)
			}
		})
	}))
	t.Cleanup(server.Close)
	t.Setenv(kube.KubectlCacheDirEnv, t.TempDir())
	configPath := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: %s
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user: {}
`, server.URL)), 0o600))
	api.kubeOpts = common.KubeConnectionOptions{KubeConfigPaths: []string{configPath}, KubeRequestTimeout: 5 * time.Second}

	return api
}

type chartCopyTestImage struct {
	Children []ocispec.Descriptor
	Desc     ocispec.Descriptor
	Ref      string
}

type chartCopyTestTarEntry struct {
	Data []byte
	Name string
}

func newChartRenderInstallChart(t *testing.T, api *progressPhasesAPI) string {
	t.Helper()

	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte(addProgressPhasesConfigMap(t, api)), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "crds"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "crds", "crd.yaml"), []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
spec:
  group: example.com
  names:
    kind: Widget
    plural: widgets
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
`), 0o600))

	return chartPath
}

func newLatestRevisionSummary(namespace, name string, version int) release.RevisionSummary {
	return release.RevisionSummary{
		Revision: release.Revision{Namespace: namespace, Name: name, Version: version, Status: "deployed"},
		Summary:  newTestReleaseSummary(),
	}
}

func newTriggersReleaseChart(t *testing.T, api *progressPhasesAPI, annotations map[string]string) string {
	t.Helper()

	cm := corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "progress-resource", Namespace: "progress-ns",
			Annotations: lo.Assign(map[string]string{
				"meta.helm.sh/release-name": "progress-release", "meta.helm.sh/release-namespace": "progress-ns",
				"werf.io/delete-policy": "before-creation",
			}, annotations),
			Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"},
		},
		Data: map[string]string{"key": "value"},
	}
	body, err := json.Marshal(cm)
	require.NoError(t, err)
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.configMaps[cm.Name] = cm
	})
	addProgressPhaseRevision(t, api, 1, string(body))

	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), body, 0o600))

	return chartPath
}

// pushChartCopyTestImage pushes an image with a manifest per architecture, under an index if there
// are several, and returns it.
func pushChartCopyTestImage(t *testing.T, repo, tag string, architectures ...string) chartCopyTestImage {
	t.Helper()

	ctx := context.Background()
	repository := newChartCopyTestRepository(t, repo)

	var manifests []ocispec.Descriptor
	for _, architecture := range architectures {
		configDesc, err := oras.PushBytes(ctx, repository, ocispec.MediaTypeImageConfig, []byte(fmt.Sprintf(`{"architecture":%q,"os":"linux","tag":%q}`, architecture, tag)))
		require.NoError(t, err)

		layerDesc, err := oras.PushBytes(ctx, repository, ocispec.MediaTypeImageLayerGzip, gzipChartCopyTestData(t, []byte(repo+tag+architecture)))
		require.NoError(t, err)

		manifestData, err := json.Marshal(ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    configDesc,
			Layers:    []ocispec.Descriptor{layerDesc},
		})
		require.NoError(t, err)

		manifestDesc, err := oras.PushBytes(ctx, repository, ocispec.MediaTypeImageManifest, manifestData)
		require.NoError(t, err)

		manifestDesc.Platform = &ocispec.Platform{OS: "linux", Architecture: architecture}
		manifests = append(manifests, manifestDesc)
	}

	if len(manifests) == 1 {
		require.NoError(t, repository.Tag(ctx, manifests[0], tag))

		return chartCopyTestImage{Desc: manifests[0], Ref: fmt.Sprintf("%s:%s@%s", repo, tag, manifests[0].Digest)}
	}

	indexData, err := json.Marshal(ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: manifests,
	})
	require.NoError(t, err)

	indexDesc, err := oras.TagBytes(ctx, repository, ocispec.MediaTypeImageIndex, indexData, tag)
	require.NoError(t, err)

	return chartCopyTestImage{Children: manifests, Desc: indexDesc, Ref: fmt.Sprintf("%s:%s@%s", repo, tag, indexDesc.Digest)}
}

// requireChartCopyTestImage checks that the image is in repo under tag with all its manifests.
func requireChartCopyTestImage(t *testing.T, repo, tag string, image chartCopyTestImage) {
	t.Helper()

	ctx := context.Background()
	repository := newChartCopyTestRepository(t, repo)

	desc, err := repository.Resolve(ctx, tag)
	require.NoError(t, err)
	assert.Equal(t, image.Desc.Digest, desc.Digest)

	for _, child := range image.Children {
		exists, err := repository.Exists(ctx, child)
		require.NoError(t, err)
		assert.True(t, exists, child.Digest.String())
	}
}

// rewriteChartCopyTestArchiveIndex copies the uncompressed OCI image layout archive with its
// index.json changed by edit.
func rewriteChartCopyTestArchiveIndex(t *testing.T, archivePath, destinationPath string, edit func(index *ocispec.Index)) {
	t.Helper()

	archiveFile, err := os.Open(archivePath)
	require.NoError(t, err)

	defer archiveFile.Close()

	var entries []chartCopyTestTarEntry

	tarReader := tar.NewReader(archiveFile)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)

		data, err := io.ReadAll(tarReader)
		require.NoError(t, err)

		if header.Name == ocispec.ImageIndexFile {
			var index ocispec.Index
			require.NoError(t, json.Unmarshal(data, &index))
			edit(&index)

			data, err = json.Marshal(index)
			require.NoError(t, err)
		}

		entries = append(entries, chartCopyTestTarEntry{Data: data, Name: header.Name})
	}

	require.NoError(t, os.WriteFile(destinationPath, tarChartCopyTestEntries(t, entries), 0o644))
}

// writeWerfChartCopyTestArchive writes a werf 3 bundle archive with the chart and a docker-save
// tarball per image tag.
func writeWerfChartCopyTestArchive(t *testing.T, chartData []byte, imageTarballs map[string][]byte) string {
	t.Helper()

	files := map[string][]byte{werfArchiveChartFileName: chartData}
	for tag, tarball := range imageTarballs {
		files[werfArchiveImagesDirName+"/"+tag+".tar.gz"] = gzipChartCopyTestData(t, tarball)
	}

	archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, gzipChartCopyTestData(t, tarChartCopyTestFiles(t, files)), 0o644))

	return archivePath
}

func addProgressPhaseRevision(t *testing.T, api *progressPhasesAPI, revision int, manifest string) {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"name": "progress-release", "namespace": "progress-ns", "version": revision,
		"info": map[string]any{"status": "deployed"}, "manifest": manifest,
		"chart": map[string]any{"metadata": map[string]any{"apiVersion": "v2", "name": "progress-chart", "version": "1.0.0"}},
	})
	require.NoError(t, err)

	key := fmt.Sprintf("sh.helm.release.v1.progress-release.v%d", revision)
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.configMaps[key] = corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: key, Namespace: "progress-ns", Labels: map[string]string{
				"owner": "helm", "name": "progress-release", "version": fmt.Sprint(revision), "status": "deployed",
			}},
			Data: map[string]string{"release": base64.StdEncoding.EncodeToString(body)},
		}
	})
}

func addProgressPhasesConfigMap(t *testing.T, api *progressPhasesAPI) string {
	t.Helper()

	cm := corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "progress-resource", Namespace: "progress-ns",
			Annotations: map[string]string{"meta.helm.sh/release-name": "progress-release", "meta.helm.sh/release-namespace": "progress-ns"},
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "Helm"},
		},
		Data: map[string]string{"key": "value"},
	}
	body, err := json.Marshal(cm)
	require.NoError(t, err)
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.configMaps[cm.Name] = cm
	})

	return string(body)
}

// buildChartCopyTestChart returns a chart archive with the files under the app directory.
func buildChartCopyTestChart(t *testing.T, files map[string]string) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(buf)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, name := range lo.Keys(files) {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "app/" + name, Mode: 0o644, Size: int64(len(files[name])), Typeflag: tar.TypeReg}))
		_, err := tarWriter.Write([]byte(files[name]))
		require.NoError(t, err)
	}

	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	return buf.Bytes()
}

func chartCopyTestBlobPath(dgst digest.Digest) string {
	return path.Join(ocispec.ImageBlobsDir, dgst.Algorithm().String(), dgst.Encoded())
}

func collectProgressPhaseReports(ch <-chan progrep.ProgressReport) <-chan []progrep.ProgressReport {
	done := make(chan []progrep.ProgressReport, 1)
	go func() {
		var reports []progrep.ProgressReport
		for report := range ch {
			reports = append(reports, report)
		}

		done <- reports
	}()

	return done
}

func collectSlowProgressPhaseReports(ch <-chan progrep.ProgressReport) <-chan []progrep.ProgressReport {
	done := make(chan []progrep.ProgressReport, 1)
	go func() {
		time.Sleep(500 * time.Millisecond)

		var reports []progrep.ProgressReport
		for report := range ch {
			reports = append(reports, report)

			time.Sleep(20 * time.Millisecond)
		}

		done <- reports
	}()

	return done
}

func encryptSecretValuesTypesFile(t *testing.T, outputPath string) {
	t.Helper()

	require.NoError(t, SecretValuesFileEncrypt(context.Background(), "testdata/secret-values-types/plain-secret-values.yaml", SecretValuesFileEncryptOptions{
		OutputFilePath: outputPath,
		SecretKey:      secretValuesTypesSecretKey,
		TempDirPath:    t.TempDir(),
	}))
}

func gzipChartCopyTestData(t *testing.T, data []byte) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(buf)
	_, err := gzipWriter.Write(data)
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())

	return buf.Bytes()
}

func newChartCopyTestOptions(t *testing.T) ChartCopyOptions {
	t.Helper()

	return ChartCopyOptions{
		ChartRepoConnectionOptions: common.ChartRepoConnectionOptions{ChartRepoInsecure: true},
		RegistryCredentialsPath:    filepath.Join(t.TempDir(), "config.json"),
		TempDirPath:                t.TempDir(),
	}
}

func newChartCopyTestRegistry(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	host := listener.Addr().String()
	require.NoError(t, listener.Close())

	config := &configuration.Configuration{}
	config.HTTP.Addr = host
	config.HTTP.DrainTimeout = time.Second
	config.Storage = configuration.Storage{"inmemory": configuration.Parameters{}}
	config.Log.Level = "fatal"
	config.Log.AccessLog.Disabled = true
	config.Validation.Manifests.URLs.Allow = []string{`^https://example\.com/`}

	t.Setenv("OTEL_TRACES_EXPORTER", "none")

	dockerRegistry, err := distributionregistry.NewRegistry(context.Background(), config)
	require.NoError(t, err)

	go func() {
		_ = dockerRegistry.ListenAndServe()
	}()

	t.Cleanup(func() {
		_ = dockerRegistry.Shutdown(context.Background())
	})

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + host + "/v2/")
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 50*time.Millisecond)

	return host
}

func newChartCopyTestRegistryClient(t *testing.T) *registry.Client {
	t.Helper()

	client, err := registry.NewClient(registry.ClientOptPlainHTTP(), registry.ClientOptWriter(io.Discard), registry.ClientOptCredentialsFile(filepath.Join(t.TempDir(), "config.json")))
	require.NoError(t, err)

	return client
}

func newChartCopyTestRepository(t *testing.T, repo string) *remote.Repository {
	t.Helper()

	repository, err := remote.NewRepository(repo)
	require.NoError(t, err)

	repository.PlainHTTP = true

	return repository
}

func newChartRenderInstallContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func newChartRenderInstallOptions(t *testing.T, api *progressPhasesAPI, chartPath string, renderResult *ChartRenderResultV2) ReleaseInstallOptions {
	t.Helper()

	return ReleaseInstallOptions{
		KubeConnectionOptions:   api.kubeOpts,
		Chart:                   chartPath,
		LegacyChartRenderResult: renderResult,
		LegacyNoReleaseLock:     true,
		NoCreateNamespace:       true,
		NoShowNotes:             true,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
			ExtraLabels:               map[string]string{"install-label": "install"},
			NoInstallStandaloneCRDs:   true,
			ReleaseStorageDriver:      "configmap",
			ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		},
		TempDirPath:     t.TempDir(),
		TrackingOptions: common.TrackingOptions{NoProgressTablePrint: true},
	}
}

func newChartRenderInstallRenderOptions(t *testing.T, api *progressPhasesAPI, chartPath, releaseName string) ChartRenderOptions {
	t.Helper()

	return ChartRenderOptions{
		KubeConnectionOptions: api.kubeOpts,
		Chart:                 chartPath,
		OutputFilePath:        filepath.Join(t.TempDir(), "output.yaml"),
		ReleaseName:           releaseName,
		ReleaseNamespace:      "progress-ns",
		ReleaseStorageDriver:  "configmap",
		Remote:                true,
		TempDirPath:           t.TempDir(),
	}
}

func newMetadataReleaseListStorager(revisions []release.Revision, summaries map[string]*release.ReleaseSummary, loadErrs ...error) *latestReleaseListStorager {
	return &latestReleaseListStorager{
		loads:     kdutil.NewConcurrent(&latestReleaseListLoads{errs: loadErrs}),
		revisions: revisions,
		summaries: summaries,
	}
}

func newProgressPhasesChart(t *testing.T) string {
	t.Helper()

	path := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(path, "Chart.yaml"), []byte("apiVersion: v2\nname: progress-chart\nversion: 1.0.0\n"), 0o600))

	return path
}

func newSecretValuesTypesChart(t *testing.T) string {
	t.Helper()

	path := t.TempDir()
	require.NoError(t, os.CopyFS(path, os.DirFS("testdata/secret-values-types/chart")))

	return path
}

func newTestReleaseSummary() *release.ReleaseSummary {
	return &release.ReleaseSummary{
		Annotations: map[string]string{"managed-by": "deckhouse"},
		Chart:       &release.ReleaseSummaryChart{AppVersion: "4.5.6", Name: "mychart", Version: "1.2.3"},
	}
}

// newWerfDataChart copies the werf-data test chart and gives it a .werf-data/metadata.yaml with
// the given contents.
func newWerfDataChart(t *testing.T, metadata string) string {
	t.Helper()

	path := t.TempDir()
	require.NoError(t, os.CopyFS(path, os.DirFS("testdata/werf-data/chart")))
	require.NoError(t, os.MkdirAll(filepath.Join(path, werfdata.DirName), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, werfdata.DirName, werfdata.MetadataFileName), []byte(metadata), 0o600))

	return path
}

func progressPhaseReports(t *testing.T, ch <-chan progrep.ProgressReport) []progrep.ProgressReport {
	t.Helper()

	var reports []progrep.ProgressReport
	for {
		select {
		case report, ok := <-ch:
			if !ok {
				require.NotEmpty(t, reports)

				return reports
			}

			reports = append(reports, report)
		default:
			t.Fatal("public action did not close its progress report channel")
			return nil
		}
	}
}

func progressPhaseSequence(reports []progrep.ProgressReport) []progrep.PhaseKind {
	var phases []progrep.PhaseKind
	for _, report := range reports {
		if len(phases) == 0 || phases[len(phases)-1] != report.CurrentPhase {
			phases = append(phases, report.CurrentPhase)
		}
	}

	return phases
}

func readChartCopyTestArchiveIndex(t *testing.T, archivePath string) ocispec.Index {
	t.Helper()

	data, err := os.ReadFile(archivePath)
	require.NoError(t, err)

	if gzipReader, err := gzip.NewReader(bytes.NewReader(data)); err == nil {
		data, err = io.ReadAll(gzipReader)
		require.NoError(t, err)
	}

	var (
		index        ocispec.Index
		hasOCILayout bool
	)

	tarReader := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err)

		switch header.Name {
		case ocispec.ImageLayoutFile:
			hasOCILayout = true
		case ocispec.ImageIndexFile:
			require.NoError(t, json.NewDecoder(tarReader).Decode(&index))
		}
	}

	require.True(t, hasOCILayout)

	return index
}

func readChartCopyTestChartFiles(t *testing.T, chartData []byte) map[string]string {
	t.Helper()

	archivePath := filepath.Join(t.TempDir(), "chart.tgz")
	require.NoError(t, os.WriteFile(archivePath, chartData, 0o644))

	return readChartPackTestArchiveFiles(t, archivePath)
}

func readChartCopyTestTarFiles(t *testing.T, archivePath string) map[string][]byte {
	t.Helper()

	archiveFile, err := os.Open(archivePath)
	require.NoError(t, err)

	defer archiveFile.Close()

	files := map[string][]byte{}

	tarReader := tar.NewReader(archiveFile)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return files
		}

		require.NoError(t, err)

		files[header.Name], err = io.ReadAll(tarReader)
		require.NoError(t, err)
	}
}

func readWerfDataTestMetadata(t *testing.T) string {
	t.Helper()

	metadata, err := os.ReadFile("testdata/werf-data/metadata.yaml")
	require.NoError(t, err)

	return string(metadata)
}

func renderedValuesJSON(t *testing.T, res *ChartRenderResultV2) string {
	t.Helper()

	require.Len(t, res.Resources, 1)

	valuesJSON, found, err := unstructured.NestedString(res.Resources[0].Unstruct.Object, "data", "values")
	require.NoError(t, err)
	require.True(t, found)

	return valuesJSON
}

// setWerfDataMergedClusterConfigMap puts the werf-data test chart's ConfigMap into the cluster as
// installing it with the werf-data test metadata and caller options produces it, so the install
// has nothing to apply.
func setWerfDataMergedClusterConfigMap(api *progressPhasesAPI) {
	annotations := lo.Assign(map[string]string{
		"meta.helm.sh/release-name": "progress-release", "meta.helm.sh/release-namespace": "progress-ns",
	}, werfDataMergedAnnotations, werfDataMergedRuntimeAnnotations)
	labels := lo.Assign(map[string]string{"app.kubernetes.io/managed-by": "Helm"}, werfDataMergedLabels)

	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.configMaps["progress-resource"] = corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "progress-resource", Namespace: "progress-ns", Annotations: annotations, Labels: labels,
			},
			Data: map[string]string{"key": "value"},
		}
	})
}

func signChartCopyTestChart(t *testing.T, chartData []byte) []byte {
	t.Helper()

	keyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.secret")
	require.NoError(t, err)

	archivePath := filepath.Join(t.TempDir(), "app-1.0.0.tgz")
	require.NoError(t, os.WriteFile(archivePath, chartData, 0o644))
	require.NoError(t, (&helmaction.Package{Key: "helm-test", Keyring: keyringPath}).Clearsign(archivePath))

	provData, err := os.ReadFile(archivePath + ".prov")
	require.NoError(t, err)

	return provData
}

// sparseChartCopyTestImageTarball returns a docker-save tarball whose layer is a GNU PAX sparse
// file of realSize bytes stored in about a kilobyte. Go can't write sparse files, so the PAX header
// is written as a regular file and then retyped.
func sparseChartCopyTestImageTarball(t *testing.T, realSize int64) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	tarWriter := tar.NewWriter(buf)

	for _, entry := range []chartCopyTestTarEntry{
		{Data: []byte(`[{"Config":"config.json","Layers":["layer.tar"]}]`), Name: "manifest.json"},
		{Data: []byte(`{"architecture":"amd64","os":"linux"}`), Name: "config.json"},
	} {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: entry.Name, Mode: 0o644, Size: int64(len(entry.Data)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}))
		_, err := tarWriter.Write(entry.Data)
		require.NoError(t, err)
	}

	paxRecord := func(key, value string) string {
		record := " " + key + "=" + value + "\n"

		size := len(record) + len(strconv.Itoa(len(record)))
		if len(strconv.Itoa(size)) != len(strconv.Itoa(len(record))) {
			size++
		}

		return strconv.Itoa(size) + record
	}
	pax := paxRecord("GNU.sparse.major", "1") + paxRecord("GNU.sparse.minor", "0") + paxRecord("GNU.sparse.name", "layer.tar") + paxRecord("GNU.sparse.realsize", strconv.FormatInt(realSize, 10))

	require.NoError(t, tarWriter.Flush())

	paxHeaderOffset := buf.Len()

	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "pax", Mode: 0o644, Size: int64(len(pax)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}))
	_, err := tarWriter.Write([]byte(pax))
	require.NoError(t, err)

	data := make([]byte, 513)
	copy(data, "1\n0\n1\n")
	data[512] = 'x'
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "GNUSparseFile.0/layer.tar", Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}))
	_, err = tarWriter.Write(data)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())

	result := buf.Bytes()
	header := result[paxHeaderOffset : paxHeaderOffset+512]
	header[156] = tar.TypeXHeader
	copy(header[148:156], "        ")

	var checksum int
	for _, b := range header {
		checksum += int(b)
	}

	copy(header[148:156], fmt.Sprintf("%06o\x00 ", checksum))

	return result
}

// tarChartCopyTestEntries returns a tar with the entries in order, duplicates included.
func tarChartCopyTestEntries(t *testing.T, entries []chartCopyTestTarEntry) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	tarWriter := tar.NewWriter(buf)

	for _, entry := range entries {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: entry.Name, Mode: 0o644, Size: int64(len(entry.Data)), Typeflag: tar.TypeReg}))
		_, err := tarWriter.Write(entry.Data)
		require.NoError(t, err)
	}

	require.NoError(t, tarWriter.Close())

	return buf.Bytes()
}

func tarChartCopyTestFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	tarWriter := tar.NewWriter(buf)

	for _, name := range lo.Keys(files) {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(files[name])), Typeflag: tar.TypeReg}))
		_, err := tarWriter.Write(files[name])
		require.NoError(t, err)
	}

	require.NoError(t, tarWriter.Close())

	return buf.Bytes()
}

func verifyChartCopyTestSignature(t *testing.T, chartData, provData []byte, archiveName string) {
	t.Helper()

	publicKeyringPath, err := filepath.Abs("../helm/pkg/cmd/testdata/helm-test-key.pub")
	require.NoError(t, err)

	signatory, err := provenance.NewFromKeyring(publicKeyringPath, "")
	require.NoError(t, err)

	_, err = signatory.Verify(chartData, provData, archiveName)
	require.NoError(t, err)
}
