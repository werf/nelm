package release

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/time"
	"github.com/werf/nelm/internal/resource"
)

func NewLegacyReleaseFromRelease(rel *Release) (*helmrelease.Release, error) {
	var legacyHooks []*helmrelease.Hook
	for _, res := range rel.HookResources() {
		if legacyHook, err := hookResourceToLegacyHook(res); err != nil {
			return nil, fmt.Errorf("error converting hook resource to legacy hook for release %q (namespace: %q, revision: %d): %w", rel.Name(), rel.Namespace(), rel.Revision(), err)
		} else {
			legacyHooks = append(legacyHooks, legacyHook)
		}
	}

	var generalResourcesManifests []string
	for _, res := range rel.GeneralResources() {
		if result, err := yaml.Marshal(res.Unstructured().UnstructuredContent()); err != nil {
			return nil, fmt.Errorf("error marshaling general resource %q for release %q (namespace: %q, revision: %d): %w", res.HumanID(), rel.Name(), rel.Namespace(), rel.Revision(), err)
		} else if res.FilePath() != "" {
			manifest := "# Source: " + res.FilePath() + "\n" + string(result)
			generalResourcesManifests = append(generalResourcesManifests, manifest)
		} else {
			generalResourcesManifests = append(generalResourcesManifests, string(result))
		}
	}

	legacyRel := &helmrelease.Release{
		Name:      rel.Name(),
		Namespace: rel.Namespace(),
		Version:   rel.Revision(),
		Info: &helmrelease.Info{
			Annotations:   rel.InfoAnnotations(),
			FirstDeployed: time.Time{Time: rel.FirstDeployed()},
			LastDeployed:  time.Time{Time: rel.LastDeployed()},
			Status:        rel.Status(),
			Notes:         rel.Notes(),
		},
		Hooks:    legacyHooks,
		Manifest: strings.Join(generalResourcesManifests, "\n---\n"),
		Config:   rel.OverrideValues(),
		Chart:    rel.LegacyChart(),
		Labels:   rel.Labels(),
	}

	return legacyRel, nil
}

func hookResourceToLegacyHook(res *resource.HookResource) (*helmrelease.Hook, error) {
	var deletePolicies []helmrelease.HookDeletePolicy
	if res.Recreate() {
		deletePolicies = append(deletePolicies, helmrelease.HookBeforeHookCreation)
	}
	if res.DeleteOnSucceeded() {
		deletePolicies = append(deletePolicies, helmrelease.HookSucceeded)
	}
	if res.DeleteOnFailed() {
		deletePolicies = append(deletePolicies, helmrelease.HookFailed)
	}

	var events []helmrelease.HookEvent
	if res.OnPreInstall() {
		events = append(events, helmrelease.HookPreInstall)
	}
	if res.OnPostInstall() {
		events = append(events, helmrelease.HookPostInstall)
	}
	if res.OnPreUpgrade() {
		events = append(events, helmrelease.HookPreUpgrade)
	}
	if res.OnPostUpgrade() {
		events = append(events, helmrelease.HookPostUpgrade)
	}
	if res.OnPreRollback() {
		events = append(events, helmrelease.HookPreRollback)
	}
	if res.OnPostRollback() {
		events = append(events, helmrelease.HookPostRollback)
	}
	if res.OnPreDelete() {
		events = append(events, helmrelease.HookPreDelete)
	}
	if res.OnPostDelete() {
		events = append(events, helmrelease.HookPostDelete)
	}
	if res.OnTest() {
		events = append(events, helmrelease.HookTest)
	}

	var manifest string
	if result, err := yaml.Marshal(res.Unstructured().UnstructuredContent()); err != nil {
		return nil, fmt.Errorf("error marshaling hook resource %q: %w", res.HumanID(), err)
	} else if res.FilePath() != "" {
		manifest = "# Source: " + res.FilePath() + "\n" + string(result)
	} else {
		manifest = string(result)
	}

	legacyHook := &helmrelease.Hook{
		Name:           res.Name(),
		Kind:           res.GroupVersionKind().Kind,
		Path:           res.FilePath(),
		Manifest:       manifest,
		Events:         events,
		Weight:         res.Weight(),
		DeletePolicies: deletePolicies,
	}

	return legacyHook, nil
}
