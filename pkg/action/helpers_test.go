package action

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/scheme"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/kube"
	"github.com/werf/nelm/v2/pkg/legacy/progrep"
	"github.com/werf/nelm/v2/pkg/release"
)

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
	summaries        map[string]*release.ReleaseSummary
}

func (s *latestReleaseListStorager) LatestRevisions(ctx context.Context, opts release.LatestRevisionsOptions) ([]release.Revision, error) {
	s.selector = opts.LabelSelector
	if s.err != nil {
		return nil, s.err
	}

	return s.revisions, nil
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
	configMaps            map[string]corev1.ConfigMap
	deleteNamespaceStatus int
	namespaceDeleted      bool
	requests              []string
	retainNamespace       bool
	trackNamespaceStatus  int
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
					s.configMaps[cm.Name] = cm
					response = cm
				case http.MethodPatch:
					assert.Equal(t, "All", r.URL.Query().Get("dryRun"))

					var cm corev1.ConfigMap
					if err := json.NewDecoder(r.Body).Decode(&cm); err != nil {
						t.Errorf("decode dry-run ConfigMap: %s", err)

						status = http.StatusBadRequest

						break
					}

					cm.Namespace = "progress-ns"
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

func newLatestRevisionSummary(namespace, name string, version int) release.RevisionSummary {
	return release.RevisionSummary{
		Revision: release.Revision{Namespace: namespace, Name: name, Version: version, Status: "deployed"},
		Summary:  newTestReleaseSummary(),
	}
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

func newTestReleaseSummary() *release.ReleaseSummary {
	return &release.ReleaseSummary{
		Annotations: map[string]string{"managed-by": "deckhouse"},
		Chart:       &release.ReleaseSummaryChart{AppVersion: "4.5.6", Name: "mychart", Version: "1.2.3"},
	}
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
