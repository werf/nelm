package release

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	legacyRelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	helmtime "helm.sh/helm/v3/pkg/time"
	"helm.sh/helm/v3/pkg/werf/resourcev2"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

// FIXME(ilya-lesikov): some legacyHook options missing
func NewRelease(name, namespace string, revision int, values map[string]interface{}, legacyChart *chart.Chart, standaloneCRDsManifests, hookManifests, generalManifests []string, notes string, opts NewReleaseOptions) *Release {
	return &Release{
		name:                    name,
		namespace:               namespace,
		revision:                revision,
		values:                  values,
		chart:                   legacyChart,
		standaloneCRDsManifests: standaloneCRDsManifests,
		hookManifests:           hookManifests,
		generalManifests:        generalManifests,
		notes:                   notes,
		mapper:                  opts.Mapper,
		discoveryClient:         opts.DiscoveryClient,
		status:                  legacyRelease.StatusUnknown,
	}
}

type NewReleaseOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type Release struct {
	name                    string
	namespace               string
	revision                int
	values                  map[string]interface{}
	chart                   *chart.Chart
	standaloneCRDsManifests []string
	hookManifests           []string
	generalManifests        []string
	notes                   string
	mapper                  meta.ResettableRESTMapper
	discoveryClient         discovery.CachedDiscoveryInterface

	status        legacyRelease.Status
	firstDeployed time.Time
	lastDeployed  time.Time

	loadOnce sync.Once

	standaloneCRDs   []*resourcev2.LocalStandaloneCRD
	hookCRDs         []*resourcev2.LocalHookCRD
	hookResources    []*resourcev2.LocalHookResource
	generalCRDs      []*resourcev2.LocalGeneralCRD
	generalResources []*resourcev2.LocalGeneralResource
}

// FIXME(ilya-lesikov): move it to constructor?
func (r *Release) Load() (*Release, error) {
	var err error
	r.loadOnce.Do(func() {
		err = r.load()
	})
	return r, err
}

func (r *Release) load() error {
	var err error
	if r.standaloneCRDs, err = resourcev2.BuildLocalStandaloneCRDsFromManifests(r.standaloneCRDsManifests, resourcev2.BuildLocalStandaloneCRDsFromManifestsOptions{
		Mapper:          r.mapper,
		DiscoveryClient: r.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building standalone CRDs from manifests: %w", err)
	}

	if r.hookCRDs, err = resourcev2.BuildLocalHookCRDsFromManifests(r.hookManifests, resourcev2.BuildLocalHookCRDsFromManifestsOptions{
		Mapper:          r.mapper,
		DiscoveryClient: r.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building hook CRDs from manifests: %w", err)
	}

	if r.hookResources, err = resourcev2.BuildLocalHookResourcesFromManifests(r.hookManifests, resourcev2.BuildLocalHookResourcesFromManifestsOptions{
		Mapper:          r.mapper,
		DiscoveryClient: r.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building hook resources from manifests: %w", err)
	}

	if r.generalCRDs, err = resourcev2.BuildLocalGeneralCRDsFromManifests(r.generalManifests, resourcev2.BuildLocalGeneralCRDsFromManifestsOptions{
		Mapper:          r.mapper,
		DiscoveryClient: r.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building general CRDs from manifests: %w", err)
	}

	if r.generalResources, err = resourcev2.BuildLocalGeneralResourcesFromManifests(r.generalManifests, resourcev2.BuildLocalGeneralResourcesFromManifestsOptions{
		Mapper:          r.mapper,
		DiscoveryClient: r.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building general resources from manifests: %w", err)
	}

	return nil
}

func (r *Release) SetStatus(status legacyRelease.Status) {
	r.status = status
}

func (r *Release) SetFirstDeployed(firstDeployed time.Time) {
	r.firstDeployed = firstDeployed
}

func (r *Release) SetLastDeployed(lastDeployed time.Time) {
	r.lastDeployed = lastDeployed
}

func (r *Release) Name() string {
	return r.name
}

func (r *Release) Namespace() string {
	return r.namespace
}

func (r *Release) Revision() int {
	return r.revision
}

func (r *Release) Values() map[string]interface{} {
	return r.values
}

func (r *Release) StandaloneCRDsManifests() []string {
	return r.standaloneCRDsManifests
}

func (r *Release) HookManifests() []string {
	return r.hookManifests
}

func (r *Release) GeneralManifests() []string {
	return r.generalManifests
}

func (r *Release) Notes() string {
	return r.notes
}

func (r *Release) StandaloneCRDs() []*resourcev2.LocalStandaloneCRD {
	return r.standaloneCRDs
}

func (r *Release) HookCRDs() []*resourcev2.LocalHookCRD {
	return r.hookCRDs
}

func (r *Release) HookResources() []*resourcev2.LocalHookResource {
	return r.hookResources
}

func (r *Release) GeneralCRDs() []*resourcev2.LocalGeneralCRD {
	return r.generalCRDs
}

func (r *Release) GeneralResources() []*resourcev2.LocalGeneralResource {
	return r.generalResources
}

func (r *Release) Status() legacyRelease.Status {
	return r.status
}

func (r *Release) FirstDeployed() time.Time {
	return r.firstDeployed
}

func (r *Release) LastDeployed() time.Time {
	return r.lastDeployed
}

// Must call Load after Clone. Values are shallow copied.
func (r *Release) Clone() *Release {
	standaloneCRDsManifests := make([]string, len(r.standaloneCRDsManifests))
	copy(standaloneCRDsManifests, r.standaloneCRDsManifests)

	hookManifests := make([]string, len(r.hookManifests))
	copy(hookManifests, r.hookManifests)

	generalManifests := make([]string, len(r.generalManifests))
	copy(generalManifests, r.generalManifests)

	values := make(map[string]interface{}, len(r.values))
	for k := range r.values {
		values[k] = r.values[k]
	}

	rel := NewRelease(r.name, r.namespace, r.revision, values, r.chart, standaloneCRDsManifests, hookManifests, generalManifests, r.notes, NewReleaseOptions{
		Mapper:          r.mapper,
		DiscoveryClient: r.discoveryClient,
	})

	rel.status = r.status
	rel.firstDeployed = r.firstDeployed
	rel.lastDeployed = r.lastDeployed

	return rel
}

func BuildReleaseFromLegacyRelease(legacyRelease *legacyRelease.Release, opts BuildReleaseFromLegacyReleaseOptions) *Release {
	var hookManifests []string
	for _, hook := range legacyRelease.Hooks {
		manifest := hook.Manifest
		if !strings.HasPrefix(manifest, "# Source: ") {
			manifest = "# Source: " + hook.Path + "\n" + manifest
		}
		hookManifests = append(hookManifests, manifest)
	}

	var generalManifests []string
	for _, manifest := range releaseutil.SplitManifests(legacyRelease.Manifest) {
		generalManifests = append(generalManifests, manifest)
	}

	return NewRelease(legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, legacyRelease.Config, legacyRelease.Chart, []string{}, hookManifests, generalManifests, legacyRelease.Info.Notes, NewReleaseOptions{
		Mapper:          opts.Mapper,
		DiscoveryClient: opts.DiscoveryClient,
	})
}

type BuildReleaseFromLegacyReleaseOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

func BuildLegacyReleaseFromRelease(rel *Release) (*legacyRelease.Release, error) {
	legacyHooks := make([]*legacyRelease.Hook, len(rel.HookManifests()))
	for _, hook := range rel.HookCRDs() {
		var deletePolicies []legacyRelease.HookDeletePolicy
		if hook.ShouldRecreate() {
			deletePolicies = append(deletePolicies, legacyRelease.HookBeforeHookCreation)
		}
		if hook.ShouldCleanupWhenSucceeded() {
			deletePolicies = append(deletePolicies, legacyRelease.HookSucceeded)
		}
		if hook.ShouldCleanupWhenFailed() {
			deletePolicies = append(deletePolicies, legacyRelease.HookFailed)
		}

		var events []legacyRelease.HookEvent
		if hook.HookPreInstall() {
			events = append(events, legacyRelease.HookPreInstall)
		}
		if hook.HookPostInstall() {
			events = append(events, legacyRelease.HookPostInstall)
		}
		if hook.HookPreUpgrade() {
			events = append(events, legacyRelease.HookPreUpgrade)
		}
		if hook.HookPostUpgrade() {
			events = append(events, legacyRelease.HookPostUpgrade)
		}
		if hook.HookPreRollback() {
			events = append(events, legacyRelease.HookPreRollback)
		}
		if hook.HookPostRollback() {
			events = append(events, legacyRelease.HookPostRollback)
		}
		if hook.HookPreDelete() {
			events = append(events, legacyRelease.HookPreDelete)
		}
		if hook.HookPostDelete() {
			events = append(events, legacyRelease.HookPostDelete)
		}
		if hook.HookTest() {
			events = append(events, legacyRelease.HookTest)
		}

		var hookManifest string
		for _, manifest := range rel.hookManifests {
			obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
			if err != nil {
				return nil, fmt.Errorf("error decoding hook manifest: %w", err)
			}

			unstructObj := obj.(*unstructured.Unstructured)

			if unstructObj.GetName() == hook.Name() &&
				unstructObj.GroupVersionKind() == hook.GroupVersionKind() &&
				(unstructObj.GetNamespace() == hook.Namespace() || unstructObj.GetNamespace() == "") {
				hookManifest = manifest
				break
			}
		}

		legacyHook := &legacyRelease.Hook{
			Name:           hook.Name(),
			Kind:           hook.GroupVersionKind().Kind,
			Path:           hook.FilePath(),
			Manifest:       hookManifest,
			Events:         events,
			Weight:         hook.Weight(),
			DeletePolicies: deletePolicies,
		}

		legacyHooks = append(legacyHooks, legacyHook)
	}

	legacyRel := &legacyRelease.Release{
		Name:      rel.Name(),
		Namespace: rel.Namespace(),
		Version:   rel.Revision(),
		Info: &legacyRelease.Info{
			FirstDeployed: helmtime.Time{Time: rel.FirstDeployed()},
			LastDeployed:  helmtime.Time{Time: rel.LastDeployed()},
			Status:        rel.Status(),
			Notes:         rel.Notes(),
		},
		Hooks:    legacyHooks,
		Manifest: strings.Join(rel.GeneralManifests(), "\n---\n"),
		Config:   rel.Values(),
		Chart:    rel.chart,
	}

	return legacyRel, nil
}
