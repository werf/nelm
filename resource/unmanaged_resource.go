package resource

import (
	"fmt"
	"regexp"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/werf/annotation"
	"helm.sh/helm/v3/pkg/werf/common"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

var _ Resourcer = (*UnmanagedResource)(nil)

func NewUnmanagedResource(u *unstructured.Unstructured, restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface) *UnmanagedResource {
	return &UnmanagedResource{
		baseResource: newBaseResource(u),
		restMapper:   restMapper,
		discClient:   discClient,
	}
}

func NewLocalUnmanagedResource(u *unstructured.Unstructured) *UnmanagedResource {
	return &UnmanagedResource{
		baseResource: newBaseResource(u),
		local:        true,
	}
}

type UnmanagedResource struct {
	*baseResource

	local      bool
	restMapper meta.ResettableRESTMapper
	discClient discovery.CachedDiscoveryInterface
}

func (r *UnmanagedResource) Validate() error {
	if err := r.baseResource.Validate(); err != nil {
		return fmt.Errorf("error validating resource: %w", err)
	}

	return nil
}

func (r *UnmanagedResource) Local() bool {
	return r.local
}

func (r *UnmanagedResource) PartOfRelease() bool {
	return false
}

func (r *UnmanagedResource) DeepCopy() Resourcer {
	return NewUnmanagedResource(r.Unstructured().DeepCopy(), r.restMapper, r.discClient)
}

func (r *UnmanagedResource) Weight() int {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationWeight); ok {
			return a.Weight()
		}
	}

	return 0
}

func (r *UnmanagedResource) KeepOnDeletion() bool {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationResourcePolicy); ok {
			policy := a.ResourcePolicy()
			if policy == common.ResourcePolicyKeep {
				return true
			} else {
				return false
			}
		}
	}

	return false
}

func (r *UnmanagedResource) ExternalDependencies() ([]*ExternalDependency, error) {
	type extDepAnno struct {
		Type      string
		Name      string
		Namespace string
	}

	extDepAnnos := map[string]*extDepAnno{}
	for key, value := range r.Unstructured().GetAnnotations() {
		switch anno := annotation.AnnotationFactory(key, value).(type) {
		case *annotation.AnnotationExternalDependencyResource:
			if _, ok := extDepAnnos[anno.ExternalDependencyId()]; !ok {
				extDepAnnos[anno.ExternalDependencyId()] = &extDepAnno{}
			}

			extDepAnnos[anno.ExternalDependencyId()].Type = anno.ExternalDependencyResourceType()
			extDepAnnos[anno.ExternalDependencyId()].Name = anno.ExternalDependencyResourceName()
		case *annotation.AnnotationExternalDependencyNamespace:
			if _, ok := extDepAnnos[anno.ExternalDependencyId()]; !ok {
				extDepAnnos[anno.ExternalDependencyId()] = &extDepAnno{}
			}

			extDepAnnos[anno.ExternalDependencyId()].Namespace = anno.ExternalDependencyNamespace()
		}
	}

	var result []*ExternalDependency
	for id, anno := range extDepAnnos {
		var extDep *ExternalDependency
		if r.Local() {
			extDep = NewLocalExternalDependency(id, anno.Type, anno.Name, anno.Namespace)
		} else {
			var err error
			extDep, err = NewExternalDependency(id, anno.Type, anno.Name, anno.Namespace, r.restMapper, r.discClient)
			if err != nil {
				return nil, fmt.Errorf("error building external dependency: %w", err)
			}
		}

		result = append(result, extDep)
	}

	return result, nil
}

func (r *UnmanagedResource) FailMode() multitrack.FailMode {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationFailMode); ok {
			return a.FailMode()
		}
	}

	return multitrack.FailWholeDeployProcessImmediately
}

func (r *UnmanagedResource) FailuresAllowed() int {
	failuresAllowedPerReplica := 1
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationFailuresAllowedPerReplica); ok {
			failuresAllowedPerReplica = a.FailuresAllowedPerReplica()
		}
	}

	if replicas, found, _ := unstructured.NestedInt64(r.Unstructured().UnstructuredContent(), "spec", "replicas"); found {
		return failuresAllowedPerReplica * int(replicas)
	} else {
		return failuresAllowedPerReplica
	}
}

func (r *UnmanagedResource) IgnoreReadinessProbeFailsForContainers() (durationByContainer map[string]time.Duration) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationIgnoreReadinessProbeFailsFor); ok {
			durationByContainer[a.ForContainer()] = a.IgnoreReadinessProbeFailsFor()
		}
	}

	return durationByContainer
}

func (r *UnmanagedResource) LogRegex() (regex *regexp.Regexp) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationLogRegex); ok {
			return a.LogRegex()
		}
	}

	return nil
}

func (r *UnmanagedResource) LogRegexesForContainers() (regexByContainer map[string]*regexp.Regexp) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationLogRegexFor); ok {
			regexByContainer[a.ForContainer()] = a.LogRegex()
		}
	}

	return regexByContainer
}

func (r *UnmanagedResource) NoActivityTimeout() (duration *time.Duration) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationNoActivityTimeout); ok {
			result := a.NoActivityTimeout()
			return &result
		}
	}

	return nil
}

func (r *UnmanagedResource) ShowLogsOnlyForContainers() (containers []string) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationShowLogsOnlyForContainers); ok {
			return a.ForContainers()
		}
	}

	return nil
}

func (r *UnmanagedResource) ShowServiceMessages() bool {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationShowServiceMessages); ok {
			return a.ShowServiceMessages()
		}
	}

	return false
}

func (r *UnmanagedResource) SkipLogs() bool {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationSkipLogs); ok {
			return a.SkipLogs()
		}
	}

	return false
}

func (r *UnmanagedResource) SkipLogsForContainers() (containers []string) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationSkipLogsForContainers); ok {
			return a.ForContainers()
		}
	}

	return nil
}

func (r *UnmanagedResource) TrackTerminationMode() multitrack.TrackTerminationMode {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationTrackTerminationMode); ok {
			return a.TrackTerminationMode()
		}
	}

	return multitrack.WaitUntilResourceReady
}

// YAML multi-documents allowed.
func BuildUnmanagedResourcesFromManifests(restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface, manifests ...string) ([]*UnmanagedResource, error) {
	var result []*UnmanagedResource

	var multidoc string
	for _, manifest := range manifests {
		multidoc = fmt.Sprintf("%s\n---\n%s", multidoc, manifest)
	}

	for _, manifest := range releaseutil.SplitManifests(multidoc) {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return nil, fmt.Errorf("error decoding manifest: %w", err)
		}

		result = append(result, NewUnmanagedResource(obj.(*unstructured.Unstructured), restMapper, discClient))
	}

	return result, nil
}

func BuildUnmanagedResourcesFromLegacyCRDs(restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface, legacyCrds ...chart.CRD) ([]*UnmanagedResource, error) {
	var result []*UnmanagedResource
	for _, legacyCrd := range legacyCrds {
		crds, err := BuildUnmanagedResourcesFromManifests(restMapper, discClient, string(legacyCrd.File.Data))
		if err != nil {
			return nil, fmt.Errorf("error building helm resources: %w", err)
		}

		result = append(result, crds...)
	}

	return result, nil
}
