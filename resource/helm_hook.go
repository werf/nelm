package resource

import (
	"fmt"
	"regexp"
	"time"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/annotation"
	"helm.sh/helm/v3/pkg/werf/common"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

var _ Resourcer = (*HelmHook)(nil)

func NewHelmHook(u *unstructured.Unstructured, filePath string, restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface) *HelmHook {
	hook := &HelmHook{
		baseResource: newBaseResource(u),
		filePath:     filePath,
		restMapper:   restMapper,
		discClient:   discClient,
	}

	return hook
}

func NewLocalHelmHook(u *unstructured.Unstructured) *HelmHook {
	hook := &HelmHook{
		baseResource: newBaseResource(u),
		local:        true,
	}

	return hook
}

type HelmHook struct {
	*baseResource

	filePath   string
	lastRun    release.HookExecution
	local      bool
	restMapper meta.ResettableRESTMapper
	discClient discovery.CachedDiscoveryInterface
}

func (h *HelmHook) Validate() error {
	if err := h.baseResource.Validate(); err != nil {
		return err
	}

	return nil
}

func (h *HelmHook) Local() bool {
	return h.local
}

func (r *HelmHook) PartOfRelease() bool {
	return true
}

func (h *HelmHook) DeepCopy() Resourcer {
	return NewHelmHook(h.Unstructured().DeepCopy(), h.filePath, h.restMapper, h.discClient)
}

func (h *HelmHook) FilePath() string {
	return h.filePath
}

func (h *HelmHook) Types() []common.HelmHookType {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationHook); ok {
			return a.HookTypes()
		}
	}

	return nil
}

func (h *HelmHook) HasType(hookType common.HelmHookType) bool {
	types := h.Types()
	if types == nil {
		return false
	}

	for _, t := range types {
		if t == hookType {
			return true
		}
	}

	return false
}

func (h *HelmHook) Weight() int {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationHookWeight); ok {
			return a.HookWeight()
		}
	}

	return 0
}

func (h *HelmHook) DeletePolicies() []common.HelmHookDeletePolicy {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationHookDeletePolicy); ok {
			return a.HookDeletePolicies()
		}
	}

	return []common.HelmHookDeletePolicy{common.DefaultHookPolicy}
}

func (h *HelmHook) HasDeletePolicy(policy common.HelmHookDeletePolicy) bool {
	for _, p := range h.DeletePolicies() {
		if p == policy {
			return true
		}
	}

	return false
}

func (h *HelmHook) KeepOnDeletion() bool {
	for key, value := range h.Unstructured().GetAnnotations() {
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

func (h *HelmHook) ExternalDependencies() ([]*ExternalDependency, error) {
	type extDepAnno struct {
		Type      string
		Name      string
		Namespace string
	}

	extDepAnnos := map[string]*extDepAnno{}
	for key, value := range h.Unstructured().GetAnnotations() {
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
		if h.Local() {
			extDep = NewLocalExternalDependency(id, anno.Type, anno.Name, anno.Namespace)
		} else {
			var err error
			extDep, err = NewExternalDependency(id, anno.Type, anno.Name, anno.Namespace, h.restMapper, h.discClient)
			if err != nil {
				return nil, fmt.Errorf("error building external dependency: %w", err)
			}
		}

		result = append(result, extDep)
	}

	return result, nil
}

func (h *HelmHook) FailMode() multitrack.FailMode {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationFailMode); ok {
			return a.FailMode()
		}
	}

	return multitrack.FailWholeDeployProcessImmediately
}

func (h *HelmHook) FailuresAllowed() int {
	failuresAllowedPerReplica := 1
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationFailuresAllowedPerReplica); ok {
			failuresAllowedPerReplica = a.FailuresAllowedPerReplica()
		}
	}

	if replicas, found, _ := unstructured.NestedInt64(h.Unstructured().UnstructuredContent(), "spec", "replicas"); found {
		return failuresAllowedPerReplica * int(replicas)
	} else {
		return failuresAllowedPerReplica
	}
}

func (h *HelmHook) IgnoreReadinessProbeFailsForContainers() (durationByContainer map[string]time.Duration) {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationIgnoreReadinessProbeFailsFor); ok {
			durationByContainer[a.ForContainer()] = a.IgnoreReadinessProbeFailsFor()
		}
	}

	return durationByContainer
}

func (h *HelmHook) LogRegex() (regex *regexp.Regexp) {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationLogRegex); ok {
			return a.LogRegex()
		}
	}

	return nil
}

func (h *HelmHook) LogRegexesForContainers() (regexByContainer map[string]*regexp.Regexp) {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationLogRegexFor); ok {
			regexByContainer[a.ForContainer()] = a.LogRegex()
		}
	}

	return regexByContainer
}

func (h *HelmHook) NoActivityTimeout() (duration *time.Duration) {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationNoActivityTimeout); ok {
			result := a.NoActivityTimeout()
			return &result
		}
	}

	return nil
}

func (h *HelmHook) ShowLogsOnlyForContainers() (containers []string) {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationShowLogsOnlyForContainers); ok {
			return a.ForContainers()
		}
	}

	return nil
}

func (h *HelmHook) ShowServiceMessages() bool {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationShowServiceMessages); ok {
			return a.ShowServiceMessages()
		}
	}

	return false
}

func (h *HelmHook) SkipLogs() bool {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationSkipLogs); ok {
			return a.SkipLogs()
		}
	}

	return false
}

func (h *HelmHook) SkipLogsForContainers() (containers []string) {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationSkipLogsForContainers); ok {
			return a.ForContainers()
		}
	}

	return nil
}

func (h *HelmHook) TrackTerminationMode() multitrack.TrackTerminationMode {
	for key, value := range h.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationTrackTerminationMode); ok {
			return a.TrackTerminationMode()
		}
	}

	return multitrack.WaitUntilResourceReady
}

func HelmHooksFromLegacyHooks(restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface, legacyHooks ...*release.Hook) ([]*HelmHook, error) {
	var result []*HelmHook
	for _, legacyHook := range legacyHooks {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(legacyHook.Manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return nil, fmt.Errorf("error decoding manifest: %w", err)
		}

		helmHook := NewHelmHook(obj.(*unstructured.Unstructured), legacyHook.Path, restMapper, discClient)

		result = append(result, helmHook)
	}

	return result, nil
}

func IsHelmHook(obj *unstructured.Unstructured) bool {
	for key, value := range obj.GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if _, ok := anno.(*annotation.AnnotationHook); ok {
			return true
		}
	}

	return false
}
