package resrcchangcalc

import (
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/pkg/plnbuilder"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcid"
	"github.com/werf/nelm/pkg/resrcinfo"
	"github.com/werf/nelm/pkg/utls"
)

const HiddenInsignificantOutput = "<hidden insignificant output>"
const HiddenInsignificantChanges = "<hidden insignificant changes>"
const HiddenSensitiveOutput = "<hidden sensitive output>"

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
		uDiff, nonEmptyDiff := utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured()))
		if !nonEmptyDiff {
			uDiff = HiddenInsignificantChanges
		}

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
			uDiff := HiddenInsignificantOutput

			changes = append(changes, &CreatedResourceChange{
				ResourceID: info.ResourceID,
				Udiff:      uDiff,
			})
		} else if update {
			uDiff, nonEmptyDiff := utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured()))
			if !nonEmptyDiff {
				uDiff = HiddenInsignificantChanges
			}

			changes = append(changes, &UpdatedResourceChange{
				ResourceID: info.ResourceID,
				Udiff:      uDiff,
			})
		} else if apply {
			uDiff := HiddenInsignificantOutput

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
		isCrd := resrc.IsCRDFromGK(info.ResourceID.GroupVersionKind().GroupKind())
		isSecret := resrc.IsSecret(info.ResourceID.GroupVersionKind().GroupKind())
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup()
		cleanupOnFailure := info.ShouldCleanupOnFailed(prevRelFailed)

		if create {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				uDiff = lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

			changes = append(changes, &CreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if recreate {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				uDiff = lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

			changes = append(changes, &RecreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if update {
			var uDiff string
			if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				if ud, nonEmpty := utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())); nonEmpty {
					uDiff = ud
				} else {
					uDiff = HiddenInsignificantChanges
				}
			}

			changes = append(changes, &UpdatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if apply {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				uDiff = lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

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
		isCrd := resrc.IsCRDFromGK(info.ResourceID.GroupVersionKind().GroupKind())
		isSecret := resrc.IsSecret(info.ResourceID.GroupVersionKind().GroupKind())
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup()
		cleanupOnFailure := info.ShouldCleanupOnFailed(prevRelFailed)

		if create {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				uDiff = lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

			changes = append(changes, &CreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if recreate {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				uDiff = lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

			changes = append(changes, &RecreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if update {
			var uDiff string
			if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				if ud, nonEmpty := utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())); nonEmpty {
					uDiff = ud
				} else {
					uDiff = HiddenInsignificantChanges
				}
			}

			changes = append(changes, &UpdatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if apply {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				uDiff = lo.Must(utls.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

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
		isCrd := resrc.IsCRDFromGK(info.ResourceID.GroupVersionKind().GroupKind())
		isSecret := resrc.IsSecret(info.ResourceID.GroupVersionKind().GroupKind())
		delete := info.ShouldDelete(curReleaseExistResourcesUIDs)

		if delete {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if isSecret {
				uDiff = HiddenSensitiveOutput
			} else {
				uDiff = lo.Must(utls.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), ""))
			}

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
