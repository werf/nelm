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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

var _ Resourcer = (*HelmResource)(nil)

func NewHelmResource(u *unstructured.Unstructured, restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface) *HelmResource {
	return &HelmResource{
		baseResource: newBaseResource(u),
		restMapper:   restMapper,
		discClient:   discClient,
	}
}

func NewLocalHelmResource(u *unstructured.Unstructured) *HelmResource {
	return &HelmResource{
		baseResource: newBaseResource(u),
		local:        true,
	}
}

type HelmResource struct {
	*baseResource

	local      bool
	restMapper meta.ResettableRESTMapper
	discClient discovery.CachedDiscoveryInterface
}

func (r *HelmResource) Validate() error {
	if err := r.baseResource.Validate(); err != nil {
		return fmt.Errorf("error validating resource: %w", err)
	}

	return nil
}

func (r *HelmResource) Local() bool {
	return r.local
}

func (r *HelmResource) PartOfRelease() bool {
	return true
}

func (r *HelmResource) DeepCopy() Resourcer {
	return NewHelmResource(r.Unstructured().DeepCopy(), r.restMapper, r.discClient)
}

func (r *HelmResource) Weight() int {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationWeight); ok {
			return a.Weight()
		}
	}

	return 0
}

func (r *HelmResource) KeepOnDeletion() bool {
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

func (r *HelmResource) ExternalDependencies() ([]*ExternalDependency, error) {
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

func (r *HelmResource) FailMode() multitrack.FailMode {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationFailMode); ok {
			return a.FailMode()
		}
	}

	return multitrack.FailWholeDeployProcessImmediately
}

func (r *HelmResource) FailuresAllowed() int {
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

func (r *HelmResource) IgnoreReadinessProbeFailsForContainers() (durationByContainer map[string]time.Duration, set bool) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationIgnoreReadinessProbeFailsFor); ok {
			durationByContainer[a.ForContainer()] = a.IgnoreReadinessProbeFailsFor()
		}
	}

	return durationByContainer, len(durationByContainer) > 0
}

func (r *HelmResource) LogRegex() (regex *regexp.Regexp, set bool) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationLogRegex); ok {
			return a.LogRegex(), true
		}
	}

	return nil, false
}

func (r *HelmResource) LogRegexesForContainers() (regexByContainer map[string]*regexp.Regexp, set bool) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationLogRegexFor); ok {
			regexByContainer[a.ForContainer()] = a.LogRegex()
		}
	}

	return regexByContainer, len(regexByContainer) > 0
}

func (r *HelmResource) NoActivityTimeout() (duration time.Duration, set bool) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationNoActivityTimeout); ok {
			return a.NoActivityTimeout(), true
		}
	}

	return 0, false
}

func (r *HelmResource) ShowLogsOnlyForContainers() (containers []string, set bool) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationShowLogsOnlyForContainers); ok {
			return a.ForContainers(), true
		}
	}

	return nil, false
}

func (r *HelmResource) ShowServiceMessages() bool {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationShowServiceMessages); ok {
			return a.ShowServiceMessages()
		}
	}

	return false
}

func (r *HelmResource) SkipLogs() bool {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationSkipLogs); ok {
			return a.SkipLogs()
		}
	}

	return false
}

func (r *HelmResource) SkipLogsForContainers() (containers []string, set bool) {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationSkipLogsForContainers); ok {
			return a.ForContainers(), true
		}
	}

	return nil, false
}

func (r *HelmResource) TrackTerminationMode() multitrack.TrackTerminationMode {
	for key, value := range r.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationTrackTerminationMode); ok {
			return a.TrackTerminationMode()
		}
	}

	return multitrack.WaitUntilResourceReady
}

// YAML multi-documents allowed.
func BuildHelmResourcesFromManifests(restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface, manifests ...string) ([]*HelmResource, error) {
	var result []*HelmResource

	var multidoc string
	for _, manifest := range manifests {
		multidoc = fmt.Sprintf("%s\n---\n%s", multidoc, manifest)
	}

	for _, manifest := range releaseutil.SplitManifests(multidoc) {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return nil, fmt.Errorf("error decoding manifest: %w", err)
		}

		result = append(result, NewHelmResource(obj.(*unstructured.Unstructured), restMapper, discClient))
	}

	return result, nil
}

func BuildHelmResourcesFromLegacyCRDs(restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface, legacyCrds ...chart.CRD) ([]*HelmResource, error) {
	var result []*HelmResource
	for _, legacyCrd := range legacyCrds {
		crds, err := BuildHelmResourcesFromManifests(restMapper, discClient, string(legacyCrd.File.Data))
		if err != nil {
			return nil, fmt.Errorf("error building helm resources: %w", err)
		}

		result = append(result, crds...)
	}

	return result, nil
}

func IsHelmResource(u *unstructured.Unstructured) bool {
	return !IsHelmHook(u) && !IsCRD(u)
}

func IsCRD(u *unstructured.Unstructured) bool {
	crdGroupKind := schema.GroupKind{
		Group: "apiextensions.k8s.io",
		Kind:  "CustomResourceDefinition",
	}

	return u.GroupVersionKind().GroupKind() == crdGroupKind
}
