package rlsdiff

import (
	"fmt"
	"hash"
	"hash/fnv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	legacyRelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/rls"
)

func ReleaseUpToDate(oldRel, newRel *rls.Release) (bool, error) {
	if oldRel.Status() != legacyRelease.StatusDeployed ||
		oldRel.Notes() != newRel.Notes() {
		return false, nil
	}

	oldHookResourcesHash := fnv.New32a()
	for _, oldHook := range oldRel.HookResources() {
		unstruct := cleanUnstruct(oldHook.Unstructured())

		if err := writeUnstructHash(unstruct, oldHookResourcesHash); err != nil {
			return false, fmt.Errorf("write old hook resource %q hash: %w", oldHook.ResourceID.HumanID(), err)
		}
	}

	newHookResourcesHash := fnv.New32a()
	for _, newHook := range newRel.HookResources() {
		unstruct := cleanUnstruct(newHook.Unstructured())

		if err := writeUnstructHash(unstruct, newHookResourcesHash); err != nil {
			return false, fmt.Errorf("write new hook resource %q hash: %w", newHook.ResourceID.HumanID(), err)
		}

	}

	if oldHookResourcesHash.Sum32() != newHookResourcesHash.Sum32() {
		return false, nil
	}

	oldGeneralResourcesHash := fnv.New32a()
	for _, oldRes := range oldRel.GeneralResources() {
		unstruct := cleanUnstruct(oldRes.Unstructured())

		if err := writeUnstructHash(unstruct, oldGeneralResourcesHash); err != nil {
			return false, fmt.Errorf("write old general resource %q hash: %w", oldRes.ResourceID.HumanID(), err)
		}
	}

	newGeneralResourcesHash := fnv.New32a()
	for _, newRes := range newRel.GeneralResources() {
		unstruct := cleanUnstruct(newRes.Unstructured())

		if err := writeUnstructHash(unstruct, newGeneralResourcesHash); err != nil {
			return false, fmt.Errorf("write new general resource %q hash: %w", newRes.ResourceID.HumanID(), err)
		}
	}

	if oldGeneralResourcesHash.Sum32() != newGeneralResourcesHash.Sum32() {
		return false, nil
	}

	return true, nil
}

func writeUnstructHash(unstruct *unstructured.Unstructured, hash hash.Hash32) error {
	if b, err := unstruct.MarshalJSON(); err != nil {
		return fmt.Errorf("unmarshal resource: %w", err)
	} else {
		hash.Write(b)
	}

	return nil
}

func cleanUnstruct(unstruct *unstructured.Unstructured) *unstructured.Unstructured {
	unstr := unstruct.DeepCopy()

	if annos := unstr.GetAnnotations(); len(annos) > 0 {
		for key := range annos {
			if strings.HasPrefix(key, "project.werf.io/") ||
				strings.Contains(key, "ci.werf.io/") ||
				key == "werf.io/version" ||
				key == "werf.io/release-channel" {
				delete(annos, key)
			}
		}

		unstr.SetAnnotations(annos)
	}

	return unstr
}
