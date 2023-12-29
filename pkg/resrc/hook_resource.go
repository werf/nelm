package resrc

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/depnd"
	"github.com/werf/nelm/resrcid"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

const TypeHookResource Type = "hook-resource"

func NewHookResource(unstruct *unstructured.Unstructured, opts HookResourceOptions) *HookResource {
	resID := resrcid.NewResourceIDFromUnstruct(unstruct, resrcid.ResourceIDOptions{
		DefaultNamespace: opts.DefaultNamespace,
		FilePath:         opts.FilePath,
		Mapper:           opts.Mapper,
	})

	return &HookResource{
		ResourceID:       resID,
		unstruct:         unstruct,
		defaultNamespace: opts.DefaultNamespace,
		mapper:           opts.Mapper,
		discoveryClient:  opts.DiscoveryClient,
	}
}

type HookResourceOptions struct {
	FilePath         string
	DefaultNamespace string
	Mapper           meta.ResettableRESTMapper
	DiscoveryClient  discovery.CachedDiscoveryInterface
}

func NewHookResourceFromManifest(manifest string, opts HookResourceFromManifestOptions) (*HookResource, error) {
	var filepath string
	if opts.FilePath != "" {
		filepath = opts.FilePath
	} else if strings.HasPrefix(manifest, "# Source: ") {
		firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
		filepath = strings.TrimPrefix(firstLine, "# Source: ")
	}

	obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
	if err != nil {
		return nil, fmt.Errorf("error decoding hook from file %q: %w", filepath, err)
	}

	unstructObj := obj.(*unstructured.Unstructured)

	resource := NewHookResource(unstructObj, HookResourceOptions{
		FilePath:         filepath,
		DefaultNamespace: opts.DefaultNamespace,
		Mapper:           opts.Mapper,
		DiscoveryClient:  opts.DiscoveryClient,
	})

	return resource, nil
}

type HookResourceFromManifestOptions struct {
	FilePath         string
	DefaultNamespace string
	Mapper           meta.ResettableRESTMapper
	DiscoveryClient  discovery.CachedDiscoveryInterface
}

type HookResource struct {
	*resrcid.ResourceID

	unstruct         *unstructured.Unstructured
	defaultNamespace string
	mapper           meta.ResettableRESTMapper
	discoveryClient  discovery.CachedDiscoveryInterface
}

func (r *HookResource) Validate() error {
	if err := validateHook(r.unstruct); err != nil {
		return fmt.Errorf("error validating hook for resource %q: %w", r.HumanID(), err)
	}

	if err := validateReplicasOnCreation(r.unstruct); err != nil {
		return fmt.Errorf("error validating replicas on creation for resource %q: %w", r.HumanID(), err)
	}

	if err := validateDeletePolicy(r.unstruct); err != nil {
		return fmt.Errorf("error validating delete policy for resource %q: %w", r.HumanID(), err)
	}

	if err := validateResourcePolicy(r.unstruct); err != nil {
		return fmt.Errorf("error validating resource policy for resource %q: %w", r.HumanID(), err)
	}

	if err := validateTrack(r.unstruct); err != nil {
		return fmt.Errorf("error validating track annotations for resource %q: %w", r.HumanID(), err)
	}

	if err := validateWeight(r.unstruct); err != nil {
		return fmt.Errorf("error validating weight for resource %q: %w", r.HumanID(), err)
	}

	if err := validateInternalDependencies(r.unstruct); err != nil {
		return fmt.Errorf("error validating internal dependencies for resource %q: %w", r.HumanID(), err)
	}

	if err := validateExternalDependencies(r.unstruct); err != nil {
		return fmt.Errorf("error validating external dependencies for resource %q: %w", r.HumanID(), err)
	}

	return nil
}

func (r *HookResource) Unstructured() *unstructured.Unstructured {
	return r.unstruct
}

func (r *HookResource) ManageableBy() ManageableBy {
	return ManageableByAnyone
}

func (r *HookResource) Type() Type {
	return TypeHookResource
}

func (r *HookResource) Recreate() bool {
	return recreate(r.unstruct)
}

func (r *HookResource) DefaultReplicasOnCreation() (replicas int, set bool) {
	return defaultReplicasOnCreation(r.unstruct)
}

func (r *HookResource) DeleteOnSucceeded() bool {
	return deleteOnSucceeded(r.unstruct)
}

func (r *HookResource) DeleteOnFailed() bool {
	return deleteOnFailed(r.unstruct)
}

func (r *HookResource) KeepOnDelete() bool {
	return keepOnDelete(r.unstruct)
}

func (r *HookResource) FailMode() multitrack.FailMode {
	return failMode(r.unstruct)
}

func (r *HookResource) FailuresAllowed() int {
	return failuresAllowed(r.unstruct)
}

func (r *HookResource) IgnoreReadinessProbeFailsForContainers() (durationByContainer map[string]time.Duration, set bool) {
	return ignoreReadinessProbeFailsForContainers(r.unstruct)
}

func (r *HookResource) LogRegex() (regex *regexp.Regexp, set bool) {
	return logRegex(r.unstruct)
}

func (r *HookResource) LogRegexesForContainers() (regexByContainer map[string]*regexp.Regexp, set bool) {
	return logRegexesForContainers(r.unstruct)
}

func (r *HookResource) NoActivityTimeout() (timeout *time.Duration, set bool) {
	return noActivityTimeout(r.unstruct)
}

func (r *HookResource) ShowLogsOnlyForContainers() (containers []string, set bool) {
	return showLogsOnlyForContainers(r.unstruct)
}

func (r *HookResource) ShowServiceMessages() bool {
	return showServiceMessages(r.unstruct)
}

func (r *HookResource) SkipLogs() bool {
	return skipLogs(r.unstruct)
}

func (r *HookResource) SkipLogsForContainers() (containers []string, set bool) {
	return skipLogsForContainers(r.unstruct)
}

func (r *HookResource) TrackTerminationMode() multitrack.TrackTerminationMode {
	return trackTerminationMode(r.unstruct)
}

func (r *HookResource) Weight() int {
	return weight(r.unstruct)
}

func (r *HookResource) ManualInternalDependencies() (dependencies []*depnd.InternalDependency, set bool) {
	return manualInternalDependencies(r.unstruct, r.defaultNamespace)
}

func (r *HookResource) AutoInternalDependencies() (dependencies []*depnd.InternalDependency, set bool) {
	return autoInternalDependencies(r.unstruct, r.defaultNamespace)
}

func (r *HookResource) ExternalDependencies() (dependencies []*depnd.ExternalDependency, set bool, err error) {
	dependencies, set, err = externalDependencies(r.unstruct, r.defaultNamespace, r.mapper, r.discoveryClient)
	if err != nil {
		return nil, false, fmt.Errorf("error getting external dependencies for resource %q: %w", r.HumanID(), err)
	}

	return dependencies, set, nil
}

func (r *HookResource) OnPreInstall() bool {
	return onPreInstall(r.unstruct)
}

func (r *HookResource) OnPostInstall() bool {
	return onPostInstall(r.unstruct)
}

func (r *HookResource) OnPreUpgrade() bool {
	return onPreUpgrade(r.unstruct)
}

func (r *HookResource) OnPostUpgrade() bool {
	return onPostUpgrade(r.unstruct)
}

func (r *HookResource) OnPreRollback() bool {
	return onPreRollback(r.unstruct)
}

func (r *HookResource) OnPostRollback() bool {
	return onPostRollback(r.unstruct)
}

func (r *HookResource) OnPreDelete() bool {
	return onPreDelete(r.unstruct)
}

func (r *HookResource) OnPostDelete() bool {
	return onPostDelete(r.unstruct)
}

func (r *HookResource) OnTest() bool {
	return onTest(r.unstruct)
}

func (r *HookResource) OnPreAnything() bool {
	return onPreAnything(r.unstruct)
}

func (r *HookResource) OnPostAnything() bool {
	return onPostAnything(r.unstruct)
}
