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

	"github.com/werf/nelm/pkg/depnd"
	"github.com/werf/nelm/pkg/resrcid"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

const TypeGeneralResource Type = "general-resource"

func NewGeneralResource(unstruct *unstructured.Unstructured, opts GeneralResourceOptions) *GeneralResource {
	resID := resrcid.NewResourceIDFromUnstruct(unstruct, resrcid.ResourceIDOptions{
		DefaultNamespace: opts.DefaultNamespace,
		FilePath:         opts.FilePath,
		Mapper:           opts.Mapper,
	})

	return &GeneralResource{
		ResourceID:       resID,
		unstruct:         unstruct,
		defaultNamespace: opts.DefaultNamespace,
		mapper:           opts.Mapper,
		discoveryClient:  opts.DiscoveryClient,
	}
}

type GeneralResourceOptions struct {
	FilePath         string
	DefaultNamespace string
	Mapper           meta.ResettableRESTMapper
	DiscoveryClient  discovery.CachedDiscoveryInterface
}

func NewGeneralResourceFromManifest(manifest string, opts GeneralResourceFromManifestOptions) (*GeneralResource, error) {
	var filepath string
	if opts.FilePath != "" {
		filepath = opts.FilePath
	} else if strings.HasPrefix(manifest, "# Source: ") {
		firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
		filepath = strings.TrimPrefix(firstLine, "# Source: ")
	}

	obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
	if err != nil {
		return nil, fmt.Errorf("error decoding resource from file %q: %w", filepath, err)
	}

	unstructObj := obj.(*unstructured.Unstructured)

	resource := NewGeneralResource(unstructObj, GeneralResourceOptions{
		FilePath:         filepath,
		DefaultNamespace: opts.DefaultNamespace,
		Mapper:           opts.Mapper,
		DiscoveryClient:  opts.DiscoveryClient,
	})

	return resource, nil
}

type GeneralResourceFromManifestOptions struct {
	FilePath         string
	DefaultNamespace string
	Mapper           meta.ResettableRESTMapper
	DiscoveryClient  discovery.CachedDiscoveryInterface
}

type GeneralResource struct {
	*resrcid.ResourceID

	unstruct         *unstructured.Unstructured
	defaultNamespace string
	mapper           meta.ResettableRESTMapper
	discoveryClient  discovery.CachedDiscoveryInterface
}

func (r *GeneralResource) Validate() error {
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

	if err := validateDeployDependencies(r.unstruct); err != nil {
		return fmt.Errorf("error validating deploy dependencies for resource %q: %w", r.HumanID(), err)
	}

	if err := validateInternalDependencies(r.unstruct); err != nil {
		return fmt.Errorf("error validating internal dependencies for resource %q: %w", r.HumanID(), err)
	}

	if err := validateExternalDependencies(r.unstruct); err != nil {
		return fmt.Errorf("error validating external dependencies for resource %q: %w", r.HumanID(), err)
	}

	return nil
}

func (r *GeneralResource) Unstructured() *unstructured.Unstructured {
	return r.unstruct
}

func (r *GeneralResource) ManageableBy() ManageableBy {
	return ManageableBySingleRelease
}

func (r *GeneralResource) Type() Type {
	return TypeGeneralResource
}

func (r *GeneralResource) Recreate() bool {
	return recreate(r.unstruct)
}

func (r *GeneralResource) DefaultReplicasOnCreation() (replicas int, set bool) {
	return defaultReplicasOnCreation(r.unstruct)
}

func (r *GeneralResource) DeleteOnSucceeded() bool {
	return deleteOnSucceeded(r.unstruct)
}

func (r *GeneralResource) DeleteOnFailed() bool {
	return deleteOnFailed(r.unstruct)
}

func (r *GeneralResource) KeepOnDelete() bool {
	return keepOnDelete(r.unstruct)
}

func (r *GeneralResource) FailMode() multitrack.FailMode {
	return failMode(r.unstruct)
}

func (r *GeneralResource) FailuresAllowed() int {
	return failuresAllowed(r.unstruct)
}

func (r *GeneralResource) IgnoreReadinessProbeFailsForContainers() (durationByContainer map[string]time.Duration, set bool) {
	return ignoreReadinessProbeFailsForContainers(r.unstruct)
}

func (r *GeneralResource) LogRegex() (regex *regexp.Regexp, set bool) {
	return logRegex(r.unstruct)
}

func (r *GeneralResource) LogRegexesForContainers() (regexByContainer map[string]*regexp.Regexp, set bool) {
	return logRegexesForContainers(r.unstruct)
}

func (r *GeneralResource) NoActivityTimeout() (timeout *time.Duration, set bool) {
	return noActivityTimeout(r.unstruct)
}

func (r *GeneralResource) ShowLogsOnlyForContainers() (containers []string, set bool) {
	return showLogsOnlyForContainers(r.unstruct)
}

func (r *GeneralResource) ShowServiceMessages() bool {
	return showServiceMessages(r.unstruct)
}

func (r *GeneralResource) SkipLogs() bool {
	return skipLogs(r.unstruct)
}

func (r *GeneralResource) SkipLogsForContainers() (containers []string, set bool) {
	return skipLogsForContainers(r.unstruct)
}

func (r *GeneralResource) TrackTerminationMode() multitrack.TrackTerminationMode {
	return trackTerminationMode(r.unstruct)
}

func (r *GeneralResource) Weight() int {
	return weight(r.unstruct)
}

func (r *GeneralResource) ManualInternalDependencies() (dependencies []*depnd.InternalDependency, set bool) {
	return manualInternalDependencies(r.unstruct, r.defaultNamespace)
}

func (r *GeneralResource) AutoInternalDependencies() (dependencies []*depnd.InternalDependency, set bool) {
	return autoInternalDependencies(r.unstruct, r.defaultNamespace)
}

func (r *GeneralResource) ExternalDependencies() (dependencies []*depnd.ExternalDependency, set bool, err error) {
	dependencies, set, err = externalDependencies(r.unstruct, r.defaultNamespace, r.mapper, r.discoveryClient)
	if err != nil {
		return nil, false, fmt.Errorf("error getting external dependencies for resource %q: %w", r.HumanID(), err)
	}

	return dependencies, set, nil
}
