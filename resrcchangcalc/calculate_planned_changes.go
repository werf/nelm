package resrcchangcalc

import (
	"strings"

	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/werf/plnbuilder"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"helm.sh/helm/v3/pkg/werf/resrcinfo"
	"helm.sh/helm/v3/pkg/werf/utls"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

func CalculatePlannedChanges(
	releaseNamespaceInfo *resrcinfo.DeployableReleaseNamespaceInfo,
	standaloneCRDsInfos []*resrcinfo.DeployableStandaloneCRDInfo,
	hookResourcesInfos []*resrcinfo.DeployableHookResourceInfo,
	generalResourcesInfos []*resrcinfo.DeployableGeneralResourceInfo,
	prevReleaseGeneralResourceInfos []*resrcinfo.DeployablePrevReleaseGeneralResourceInfo,
	prevRelFailed bool,
) (
	createdChanges []*CreatedResourceChange,
	recreatedChanges []*RecreatedResourceChange,
	updatedChanges []*UpdatedResourceChange,
	appliedChanges []*AppliedResourceChange,
	deletedChanges []*DeletedResourceChange,
) {
	curReleaseExistResourcesUIDs, _ := plnbuilder.CurrentReleaseExistingResourcesUIDs(standaloneCRDsInfos, hookResourcesInfos, generalResourcesInfos)

	allChanges := make([]any, 0)

	if change, present := releaseNamespaceChange(releaseNamespaceInfo); present {
		allChanges = append(allChanges, change)
	}

	if changes, present := standaloneCRDChanges(standaloneCRDsInfos); present {
		allChanges = append(allChanges, changes...)
	}

	if changes, present := hookResourcesChanges(hookResourcesInfos, prevRelFailed); present {
		allChanges = append(allChanges, changes...)
	}

	if changes, present := generalResourcesChanges(generalResourcesInfos, prevRelFailed); present {
		allChanges = append(allChanges, changes...)
	}

	if changes, present := prevReleaseGeneralResourcesChanges(prevReleaseGeneralResourceInfos, curReleaseExistResourcesUIDs); present {
		allChanges = append(allChanges, changes...)
	}

	for _, change := range allChanges {
		switch ch := change.(type) {
		case *CreatedResourceChange:
			createdChanges = append(createdChanges, ch)
		case *RecreatedResourceChange:
			recreatedChanges = append(recreatedChanges, ch)
		case *UpdatedResourceChange:
			updatedChanges = append(updatedChanges, ch)
		case *AppliedResourceChange:
			appliedChanges = append(appliedChanges, ch)
		case *DeletedResourceChange:
			deletedChanges = append(deletedChanges, ch)
		default:
			panic("unexpected type")
		}
	}

	return createdChanges, recreatedChanges, updatedChanges, appliedChanges, deletedChanges
}

func releaseNamespaceChange(info *resrcinfo.DeployableReleaseNamespaceInfo) (change any, present bool) {
	create := info.ShouldCreate()
	update := info.ShouldUpdate()
	apply := info.ShouldApply()

	if create {
		uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

		change = &CreatedResourceChange{
			ResourceID: info.ResourceID,
			Udiff:      uDiff,
		}
	} else if update {
		uDiff := lo.Must(utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())))

		change = &UpdatedResourceChange{
			ResourceID: info.ResourceID,
			Udiff:      uDiff,
		}
	} else if apply {
		uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

		change = &AppliedResourceChange{
			ResourceID: info.ResourceID,
			Udiff:      uDiff,
		}
	}

	return change, change != nil
}

func standaloneCRDChanges(infos []*resrcinfo.DeployableStandaloneCRDInfo) (changes []any, present bool) {
	for _, info := range infos {
		create := info.ShouldCreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()

		if create {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &CreatedResourceChange{
				ResourceID: info.ResourceID,
				Udiff:      uDiff,
			})
		} else if update {
			uDiff := lo.Must(utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())))

			changes = append(changes, &UpdatedResourceChange{
				ResourceID: info.ResourceID,
				Udiff:      uDiff,
			})
		} else if apply {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &AppliedResourceChange{
				ResourceID: info.ResourceID,
				Udiff:      uDiff,
			})
		}
	}

	return changes, len(changes) > 0
}

func hookResourcesChanges(infos []*resrcinfo.DeployableHookResourceInfo, prevRelFailed bool) (changes []any, present bool) {
	for _, info := range infos {
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup()
		cleanupOnFailure := info.ShouldCleanupOnFailed(prevRelFailed)

		if create {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &CreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if recreate {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &RecreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if update {
			uDiff := lo.Must(utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())))

			changes = append(changes, &UpdatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if apply {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &AppliedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		}
	}

	return changes, len(changes) > 0
}

func generalResourcesChanges(infos []*resrcinfo.DeployableGeneralResourceInfo, prevRelFailed bool) (changes []any, present bool) {
	for _, info := range infos {
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup()
		cleanupOnFailure := info.ShouldCleanupOnFailed(prevRelFailed)

		if create {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &CreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if recreate {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &RecreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if update {
			uDiff := lo.Must(utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())))

			changes = append(changes, &UpdatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if apply {
			uDiff := lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))

			changes = append(changes, &AppliedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		}
	}

	return changes, len(changes) > 0
}

func prevReleaseGeneralResourcesChanges(infos []*resrcinfo.DeployablePrevReleaseGeneralResourceInfo, curReleaseExistResourcesUIDs []types.UID) (changes []any, present bool) {
	for _, info := range infos {
		delete := info.ShouldDelete(curReleaseExistResourcesUIDs)

		if delete {
			uDiff := lo.Must(utls.ColoredUnifiedDiff(diffableResource(info.Resource().Unstructured()), ""))

			changes = append(changes, &DeletedResourceChange{
				ResourceID: info.ResourceID,
				Udiff:      uDiff,
			})
		}
	}

	return changes, len(changes) > 0
}

func diffableResource(unstruct *unstructured.Unstructured) string {
	unstructured.RemoveNestedField(unstruct.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(unstruct.Object, "metadata", "generation")
	unstructured.RemoveNestedField(unstruct.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(unstruct.Object, "metadata", "uid")
	unstructured.RemoveNestedField(unstruct.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(unstruct.Object, "status")

	if annotations := unstruct.GetAnnotations(); len(annotations) > 0 {
		cleanedAnnotations := make(map[string]string)

		for key, val := range annotations {
			if strings.Contains(key, "werf.io") {
				continue
			}
			cleanedAnnotations[key] = val
		}

		unstruct.SetAnnotations(cleanedAnnotations)
	}

	if labels := unstruct.GetLabels(); len(labels) > 0 {
		cleanedLabels := make(map[string]string)

		for key, val := range labels {
			if strings.Contains(key, "werf.io") {
				continue
			}
			cleanedLabels[key] = val
		}

		unstruct.SetLabels(cleanedLabels)
	}

	resource := string(lo.Must(yaml.Marshal(unstruct.UnstructuredContent())))

	return resource
}

type CreatedResourceChange struct {
	*resrcid.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type RecreatedResourceChange struct {
	*resrcid.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type UpdatedResourceChange struct {
	*resrcid.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type AppliedResourceChange struct {
	*resrcid.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type DeletedResourceChange struct {
	*resrcid.ResourceID

	Udiff string
}
