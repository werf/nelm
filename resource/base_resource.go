package resource

import (
	"fmt"
	"regexp"
	"time"

	"helm.sh/helm/v3/pkg/werf/annotation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

func newBaseResource(u *unstructured.Unstructured) *baseResource {
	ref := NewReference(u.GetName(), u.GetNamespace(), u.GroupVersionKind())

	return &baseResource{
		Reference:    ref,
		unstructured: u,
	}
}

type baseResource struct {
	*Reference
	unstructured *unstructured.Unstructured
}

func (r *baseResource) FailuresAllowed() int {
	return 1
}

func (r *baseResource) LogRegex() *regexp.Regexp {
	return nil
}

func (r *baseResource) LogRegexesForContainers() map[string]*regexp.Regexp {
	return nil
}

func (r *baseResource) SkipLogsForContainers() []string {
	return nil
}

func (r *baseResource) ShowLogsOnlyForContainers() []string {
	return nil
}

func (r *baseResource) IgnoreReadinessProbeFailsForContainers() map[string]time.Duration {
	return nil
}

func (r *baseResource) TrackTerminationMode() multitrack.TrackTerminationMode {
	return multitrack.WaitUntilResourceReady
}

func (r *baseResource) FailMode() multitrack.FailMode {
	return multitrack.FailWholeDeployProcessImmediately
}

func (r *baseResource) SkipLogs() bool {
	return false
}

func (r *baseResource) ShowServiceMessages() bool {
	return false
}

func (r *baseResource) NoActivityTimeout() *time.Duration {
	return nil
}

func (r *baseResource) Validate() error {
	for key, value := range r.unstructured.GetAnnotations() {
		if err := annotation.AnnotationFactory(key, value).Validate(); err != nil {
			return fmt.Errorf("error validating annotation: %w", err)
		}
	}

	// FIXME(ilya-lesikov): make sure there is no way to only specify external dependency namespace

	return nil
}

func (r *baseResource) Name() string {
	return r.unstructured.GetName()
}

func (r *baseResource) Namespace() string {
	return r.unstructured.GetNamespace()
}

func (r *baseResource) GroupVersionKind() schema.GroupVersionKind {
	return r.unstructured.GroupVersionKind()
}

func (r *baseResource) Unstructured() *unstructured.Unstructured {
	return r.unstructured
}
