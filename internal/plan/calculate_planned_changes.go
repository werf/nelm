package plan

import (
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/internal/common"
	info "github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
)

const (
	HiddenInsignificantOutput  = "<hidden insignificant output>"
	HiddenInsignificantChanges = "<hidden insignificant changes>"
	HiddenSensitiveOutput      = "<hidden sensitive output>"
	HiddenSensitiveChanges     = "<hidden sensitive changes>"
)

func CalculatePlannedChanges(
	deployType common.DeployType,
	releaseName string,
	releaseNamespace string,
	standaloneCRDsInfos []*info.DeployableStandaloneCRDInfo,
	hookResourcesInfos []*info.DeployableHookResourceInfo,
	generalResourcesInfos []*info.DeployableGeneralResourceInfo,
	prevReleaseGeneralResourceInfos []*info.DeployablePrevReleaseGeneralResourceInfo,
	prevRelFailed bool,
) (
	createdChanges []*CreatedResourceChange,
	recreatedChanges []*RecreatedResourceChange,
	updatedChanges []*UpdatedResourceChange,
	appliedChanges []*AppliedResourceChange,
	deletedChanges []*DeletedResourceChange,
	anyChangesPlanned bool,
) {
	curReleaseExistResourcesUIDs, _ := CurrentReleaseExistingResourcesUIDs(standaloneCRDsInfos, hookResourcesInfos, generalResourcesInfos)

	allChanges := make([]any, 0)

	if changes, present := standaloneCRDChanges(standaloneCRDsInfos); present {
		allChanges = append(allChanges, changes...)
	}

	if changes, present := hookResourcesChanges(hookResourcesInfos, prevRelFailed, releaseName, releaseNamespace); present {
		allChanges = append(allChanges, changes...)
	}

	if changes, present := generalResourcesChanges(generalResourcesInfos, prevRelFailed, releaseName, releaseNamespace); present {
		allChanges = append(allChanges, changes...)
	}

	if changes, present := prevReleaseGeneralResourcesChanges(prevReleaseGeneralResourceInfos, curReleaseExistResourcesUIDs, releaseName, releaseNamespace, deployType); present {
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

	if len(allChanges) == 0 {
		return nil, nil, nil, nil, nil, false
	}

	return createdChanges, recreatedChanges, updatedChanges, appliedChanges, deletedChanges, true
}

func standaloneCRDChanges(infos []*info.DeployableStandaloneCRDInfo) (changes []any, present bool) {
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
			uDiff, nonEmptyDiff := util.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured()))
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

func hookResourcesChanges(infos []*info.DeployableHookResourceInfo, prevRelFailed bool, releaseName, releaseNamespace string) (changes []any, present bool) {
	for _, info := range infos {
		isCrd := util.IsCRDFromGK(info.ResourceID.GroupVersionKind().GroupKind())
		sensitiveInfo := resource.GetSensitiveInfo(info.ResourceID.GroupVersionKind().GroupKind(), info.Resource().Unstructured().GetAnnotations())
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup(releaseName, releaseNamespace)
		cleanupOnFailure := info.ShouldCleanupOnFailed(prevRelFailed, releaseName, releaseNamespace)

		if create {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					uDiff = HiddenSensitiveOutput
				} else {
					redactedResource := resource.RedactSensitiveData(info.Resource().Unstructured(), sensitiveInfo.SensitivePaths)
					uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(redactedResource)))
				}
			} else {
				uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
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
			} else if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					uDiff = HiddenSensitiveOutput
				} else {
					redactedResource := resource.RedactSensitiveData(info.Resource().Unstructured(), sensitiveInfo.SensitivePaths)
					uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(redactedResource)))
				}
			} else {
				uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

			changes = append(changes, &RecreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if update {
			var uDiff string
			if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					if _, nonEmpty := util.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())); nonEmpty {
						uDiff = HiddenSensitiveChanges
					} else {
						uDiff = HiddenInsignificantChanges
					}
				} else {
					redactedLive := resource.RedactSensitiveData(info.LiveResource().Unstructured(), sensitiveInfo.SensitivePaths)
					redactedNew := resource.RedactSensitiveData(info.DryApplyResource().Unstructured(), sensitiveInfo.SensitivePaths)
					if ud, nonEmpty := util.ColoredUnifiedDiff(diffableResource(redactedLive), diffableResource(redactedNew)); nonEmpty {
						uDiff = ud
					} else {
						uDiff = HiddenInsignificantChanges
					}
				}
			} else {
				if ud, nonEmpty := util.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())); nonEmpty {
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
			} else if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					uDiff = HiddenSensitiveOutput
				} else {
					redactedResource := resource.RedactSensitiveData(info.Resource().Unstructured(), sensitiveInfo.SensitivePaths)
					uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(redactedResource)))
				}
			} else {
				uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
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

func generalResourcesChanges(infos []*info.DeployableGeneralResourceInfo, prevRelFailed bool, releaseName, releaseNamespace string) (changes []any, present bool) {
	for _, info := range infos {
		isCrd := util.IsCRDFromGK(info.ResourceID.GroupVersionKind().GroupKind())
		sensitiveInfo := resource.GetSensitiveInfo(info.ResourceID.GroupVersionKind().GroupKind(), info.Resource().Unstructured().GetAnnotations())
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup(releaseName, releaseNamespace)
		cleanupOnFailure := info.ShouldCleanupOnFailed(prevRelFailed, releaseName, releaseNamespace)

		if create {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					uDiff = HiddenSensitiveOutput
				} else {
					redactedResource := resource.RedactSensitiveData(info.Resource().Unstructured(), sensitiveInfo.SensitivePaths)
					uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(redactedResource)))
				}
			} else {
				uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
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
			} else if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					uDiff = HiddenSensitiveOutput
				} else {
					redactedResource := resource.RedactSensitiveData(info.Resource().Unstructured(), sensitiveInfo.SensitivePaths)
					uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(redactedResource)))
				}
			} else {
				uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
			}

			changes = append(changes, &RecreatedResourceChange{
				ResourceID:         info.ResourceID,
				Udiff:              uDiff,
				CleanedUpOnSuccess: cleanup,
				CleanedUpOnFailure: cleanupOnFailure,
			})
		} else if update {
			var uDiff string
			if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					if _, nonEmpty := util.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())); nonEmpty {
						uDiff = HiddenSensitiveChanges
					} else {
						uDiff = HiddenInsignificantChanges
					}
				} else {
					redactedLive := resource.RedactSensitiveData(info.LiveResource().Unstructured(), sensitiveInfo.SensitivePaths)
					redactedNew := resource.RedactSensitiveData(info.DryApplyResource().Unstructured(), sensitiveInfo.SensitivePaths)
					if ud, nonEmpty := util.ColoredUnifiedDiff(diffableResource(redactedLive), diffableResource(redactedNew)); nonEmpty {
						uDiff = ud
					} else {
						uDiff = HiddenInsignificantChanges
					}
				}
			} else {
				if ud, nonEmpty := util.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), diffableResource(info.DryApplyResource().Unstructured())); nonEmpty {
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
			} else if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					uDiff = HiddenSensitiveOutput
				} else {
					redactedResource := resource.RedactSensitiveData(info.Resource().Unstructured(), sensitiveInfo.SensitivePaths)
					uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(redactedResource)))
				}
			} else {
				uDiff = lo.Must(util.ColoredUnifiedDiff("", diffableResource(info.Resource().Unstructured())))
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

func prevReleaseGeneralResourcesChanges(infos []*info.DeployablePrevReleaseGeneralResourceInfo, curReleaseExistResourcesUIDs []types.UID, releaseName, releaseNamespace string, deployType common.DeployType) (changes []any, present bool) {
	for _, info := range infos {
		isCrd := util.IsCRDFromGK(info.ResourceID.GroupVersionKind().GroupKind())
		sensitiveInfo := resource.GetSensitiveInfo(info.ResourceID.GroupVersionKind().GroupKind(), info.Resource().Unstructured().GetAnnotations())
		delete := info.ShouldDelete(curReleaseExistResourcesUIDs, releaseName, releaseNamespace, deployType)

		if delete {
			var uDiff string
			if isCrd {
				uDiff = HiddenInsignificantOutput
			} else if sensitiveInfo.IsSensitive {
				if len(sensitiveInfo.SensitivePaths) == 1 && sensitiveInfo.SensitivePaths[0] == resource.HideAll {
					uDiff = HiddenSensitiveOutput
				} else {
					redactedResource := resource.RedactSensitiveData(info.LiveResource().Unstructured(), sensitiveInfo.SensitivePaths)
					uDiff = lo.Must(util.ColoredUnifiedDiff(diffableResource(redactedResource), ""))
				}
			} else {
				uDiff = lo.Must(util.ColoredUnifiedDiff(diffableResource(info.LiveResource().Unstructured()), ""))
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
			if strings.Contains(key, "werf.io") ||
				strings.Contains(key, "helm.sh") {
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
	*id.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type RecreatedResourceChange struct {
	*id.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type UpdatedResourceChange struct {
	*id.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type AppliedResourceChange struct {
	*id.ResourceID

	Udiff              string
	CleanedUpOnSuccess bool
	CleanedUpOnFailure bool
}

type DeletedResourceChange struct {
	*id.ResourceID

	Udiff string
}
