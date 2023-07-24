package resource

import (
	"fmt"
	"regexp"
	"time"

	"helm.sh/helm/v3/pkg/werf/util"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

func NewExternalDependency(id, resourceType, name, namespace string, restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface) (*ExternalDependency, error) {
	gvr := util.ParseResourceStringToGVR(resourceType)

	gvk, err := util.ParseResourceStringtoGVK(resourceType, restMapper, discClient)
	if err != nil {
		return nil, fmt.Errorf("error parsing resource type %q: %w", resourceType, err)
	}

	ref := NewResourcedReference(name, namespace, ResourcedReferenceOptions{
		GroupVersionKind:     gvk,
		GroupVersionResource: gvr,
	})

	return &ExternalDependency{
		ResourcedReference: ref,
		id:                 id,
	}, nil
}

func NewLocalExternalDependency(id, resourceType, name, namespace string) *ExternalDependency {
	gvr := util.ParseResourceStringToGVR(resourceType)

	ref := NewResourcedReference(name, namespace, ResourcedReferenceOptions{
		GroupVersionResource: gvr,
	})

	return &ExternalDependency{
		ResourcedReference: ref,
		id:                 id,
	}
}

type ExternalDependency struct {
	*ResourcedReference

	id string
}

func (d *ExternalDependency) FailuresAllowed() int {
	return 1
}

func (d *ExternalDependency) LogRegex() *regexp.Regexp {
	return nil
}

func (d *ExternalDependency) LogRegexesForContainers() map[string]*regexp.Regexp {
	return nil
}

func (d *ExternalDependency) SkipLogsForContainers() []string {
	return nil
}

func (d *ExternalDependency) ShowLogsOnlyForContainers() []string {
	return nil
}

func (d *ExternalDependency) IgnoreReadinessProbeFailsForContainers() map[string]time.Duration {
	return nil
}

func (d *ExternalDependency) TrackTerminationMode() multitrack.TrackTerminationMode {
	return multitrack.WaitUntilResourceReady
}

func (d *ExternalDependency) FailMode() multitrack.FailMode {
	return multitrack.FailWholeDeployProcessImmediately
}

func (d *ExternalDependency) SkipLogs() bool {
	return false
}

func (d *ExternalDependency) ShowServiceMessages() bool {
	return false
}

func (d *ExternalDependency) NoActivityTimeout() *time.Duration {
	return nil
}

func (d *ExternalDependency) ID() string {
	return d.id
}
