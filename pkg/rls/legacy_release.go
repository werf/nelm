package rls

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/time"

	"github.com/werf/nelm/pkg/resrc"
)

func NewLegacyReleaseFromRelease(rel *Release) (*release.Release, error) {
	var legacyHooks []*release.Hook
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
			return nil, fmt.Errorf("error marshalling general resource %q for release %q (namespace: %q, revision: %d): %w", res.HumanID(), rel.Name(), rel.Namespace(), rel.Revision(), err)
		} else if res.FilePath() != "" {
			manifest := "# Source: " + res.FilePath() + "\n" + string(result)
			generalResourcesManifests = append(generalResourcesManifests, manifest)
		} else {
			generalResourcesManifests = append(generalResourcesManifests, string(result))
		}
	}

	legacyRel := &release.Release{
		Name:      rel.Name(),
		Namespace: rel.Namespace(),
		Version:   rel.Revision(),
		Info: &release.Info{
			FirstDeployed: time.Time{Time: rel.FirstDeployed()},
			LastDeployed:  time.Time{Time: rel.LastDeployed()},
			Status:        rel.Status(),
			Notes:         rel.Notes(),
		},
		Hooks:    legacyHooks,
		Manifest: strings.Join(generalResourcesManifests, "\n---\n"),
		Config:   rel.Values(),
		Chart:    rel.LegacyChart(),
	}

	return legacyRel, nil
}

func hookResourceToLegacyHook(res *resrc.HookResource) (*release.Hook, error) {
	var deletePolicies []release.HookDeletePolicy
	if res.Recreate() {
		deletePolicies = append(deletePolicies, release.HookBeforeHookCreation)
	}
	if res.DeleteOnSucceeded() {
		deletePolicies = append(deletePolicies, release.HookSucceeded)
	}
	if res.DeleteOnFailed() {
		deletePolicies = append(deletePolicies, release.HookFailed)
	}

	var events []release.HookEvent
	if res.OnPreInstall() {
		events = append(events, release.HookPreInstall)
	}
	if res.OnPostInstall() {
		events = append(events, release.HookPostInstall)
	}
	if res.OnPreUpgrade() {
		events = append(events, release.HookPreUpgrade)
	}
	if res.OnPostUpgrade() {
		events = append(events, release.HookPostUpgrade)
	}
	if res.OnPreRollback() {
		events = append(events, release.HookPreRollback)
	}
	if res.OnPostRollback() {
		events = append(events, release.HookPostRollback)
	}
	if res.OnPreDelete() {
		events = append(events, release.HookPreDelete)
	}
	if res.OnPostDelete() {
		events = append(events, release.HookPostDelete)
	}
	if res.OnTest() {
		events = append(events, release.HookTest)
	}

	var manifest string
	if result, err := yaml.Marshal(res.Unstructured().UnstructuredContent()); err != nil {
		return nil, fmt.Errorf("error marshalling hook resource %q: %w", res.HumanID(), err)
	} else if res.FilePath() != "" {
		manifest = "# Source: " + res.FilePath() + "\n" + string(result)
	} else {
		manifest = string(result)
	}

	legacyHook := &release.Hook{
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
