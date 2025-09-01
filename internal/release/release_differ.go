package release

import (
	"fmt"
	"hash"
	"hash/fnv"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
)

func ReleaseUpToDate(oldRel, newRel *helmrelease.Release) (bool, error) {
	if oldRel.Info.Status != helmrelease.StatusDeployed ||
		oldRel.Info.Notes != newRel.Info.Notes ||
		!reflect.DeepEqual(oldRel.Info.Annotations, newRel.Info.Annotations) ||
		!reflect.DeepEqual(oldRel.Labels, newRel.Labels) ||
		!reflect.DeepEqual(oldRel.Config, newRel.Config) {
		return false, nil
	}

	oldHookResourcesHash := fnv.New32a()
	for _, oldHook := range oldRel.Hooks {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(oldHook.Manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode old hook: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, oldHookResourcesHash); err != nil {
			return false, fmt.Errorf("write old hook hash: %w", err)
		}
	}

	newHookResourcesHash := fnv.New32a()
	for _, newHook := range newRel.Hooks {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(newHook.Manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode new hook: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, newHookResourcesHash); err != nil {
			return false, fmt.Errorf("write new hook hash: %w", err)
		}
	}

	if oldHookResourcesHash.Sum32() != newHookResourcesHash.Sum32() {
		return false, nil
	}

	oldRelManifests := releaseutil.SplitManifests(oldRel.Manifest)
	oldRegularResourcesHash := fnv.New32a()
	for _, manifest := range oldRelManifests {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode old regular resource: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, oldRegularResourcesHash); err != nil {
			return false, fmt.Errorf("write old regular resource hash: %w", err)
		}
	}

	newRelManifests := releaseutil.SplitManifests(newRel.Manifest)
	newRegularResourcesHash := fnv.New32a()
	for _, manifest := range newRelManifests {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode new regular resource: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, newRegularResourcesHash); err != nil {
			return false, fmt.Errorf("write new regular resource hash: %w", err)
		}
	}

	if oldRegularResourcesHash.Sum32() != newRegularResourcesHash.Sum32() {
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
