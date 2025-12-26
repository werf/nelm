package plan

import (
	"context"
	"fmt"
	"sort"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
)

const (
	HiddenVerboseCRDChanges    = "<hidden verbose CRD changes>"
	HiddenVerboseChanges       = "<hidden verbose changes>"
	HiddenInsignificantChanges = "<hidden insignificant changes>"
	HiddenSensitiveChanges     = "<hidden sensitive changes>"
)

type CalculatePlannedChangesOptions struct {
	DiffContextLines       int
	ShowVerboseCRDDiffs    bool
	ShowVerboseDiffs       bool
	ShowSensitiveDiffs     bool
	ShowInsignificantDiffs bool
}

type ResourceChange struct {
	// Any operations on the resource after the initial one.
	ExtraOperations []string
	// The reason for the change.
	Reason       string
	ResourceMeta *spec.ResourceMeta
	Type         string
	TypeStyle    color.Style
	Udiff        string
}

// Calculate planned changes for informational purposes. Doesn't need the full plan, just having
// Installable/DeletableResourceInfos is enough. Returns the structured result and shouldn't decide
// on how to present this data.
func CalculatePlannedChanges(installableInfos []*InstallableResourceInfo, deletableInfos []*DeletableResourceInfo, opts CalculatePlannedChangesOptions) ([]*ResourceChange, error) {
	instInfosByIter := groupInstInfosByIter(installableInfos)

	instChanges, err := buildInstChanges(instInfosByIter, opts)
	if err != nil {
		return nil, fmt.Errorf("build installable resource changes: %w", err)
	}

	sort.SliceStable(deletableInfos, func(i, j int) bool {
		return spec.ResourceMetaSortHandler(deletableInfos[i].ResourceMeta, deletableInfos[j].ResourceMeta)
	})

	delChanges, err := buildDelChanges(deletableInfos, opts)
	if err != nil {
		return nil, fmt.Errorf("build deletable resource changes: %w", err)
	}

	return append(instChanges, delChanges...), nil
}

func groupInstInfosByIter(installableInfos []*InstallableResourceInfo) [][]*InstallableResourceInfo {
	var instInfosByIter [][]*InstallableResourceInfo
	for _, instInfo := range installableInfos {
		if len(instInfosByIter) < instInfo.Iteration+1 {
			instInfosByIter = append(instInfosByIter, []*InstallableResourceInfo{})
		}

		instInfosByIter[instInfo.Iteration] = append(instInfosByIter[instInfo.Iteration], instInfo)
	}

	for _, instInfos := range instInfosByIter {
		sort.SliceStable(instInfos, func(i, j int) bool {
			return InstallableResourceInfoSortByMustInstallHandler(instInfos[i], instInfos[j])
		})
	}

	return instInfosByIter
}

func buildInstChanges(instInfosByIter [][]*InstallableResourceInfo, opts CalculatePlannedChangesOptions) ([]*ResourceChange, error) {
	var changes []*ResourceChange
	for iter, instInfos := range instInfosByIter {
		if iter == 0 {
			for _, info := range instInfos {
				var change *ResourceChange
				switch info.MustInstall {
				case ResourceInstallTypeCreate:
					var err error

					change, err = buildResourceChange(info.ResourceMeta, nil, info.LocalResource.Unstruct, info.MustDeleteOnSuccessfulInstall, "create", color.Style{color.Bold, color.Green}, opts)
					if err != nil {
						return nil, fmt.Errorf("build resource change for create: %w", err)
					}
				case ResourceInstallTypeRecreate:
					var err error

					change, err = buildResourceChange(info.ResourceMeta, nil, info.LocalResource.Unstruct, info.MustDeleteOnSuccessfulInstall, "recreate", color.Style{color.Bold, color.LightGreen}, opts)
					if err != nil {
						return nil, fmt.Errorf("build resource change for recreate: %w", err)
					}
				case ResourceInstallTypeUpdate:
					var err error

					change, err = buildResourceChange(info.ResourceMeta, info.GetResult, info.DryApplyResult, info.MustDeleteOnSuccessfulInstall, "update", color.Style{color.Bold, color.Yellow}, opts)
					if err != nil {
						return nil, fmt.Errorf("build resource change for update: %w", err)
					}
				case ResourceInstallTypeApply:
					var err error

					change, err = buildResourceChange(info.ResourceMeta, nil, info.LocalResource.Unstruct, info.MustDeleteOnSuccessfulInstall, "blind apply", color.Style{color.Bold, color.LightYellow}, opts)
					if err != nil {
						return nil, fmt.Errorf("build resource change for blind apply: %w", err)
					}

					if info.DryApplyErr != nil {
						change.Reason = fmt.Sprintf("error: %s", info.DryApplyErr)
					}
				case ResourceInstallTypeNone:
					continue
				default:
					panic("unexpected resource must install condition")
				}

				changes = append(changes, change)
			}
		} else {
			for _, info := range instInfos {
				change, found := lo.Find(changes, func(change *ResourceChange) bool {
					return change.ResourceMeta.ID() == info.ID() && info.Iteration == iter-1
				})
				if !found {
					continue
				}

				switch info.MustInstall {
				case ResourceInstallTypeCreate:
					change.ExtraOperations = append(change.ExtraOperations, "create")
				case ResourceInstallTypeRecreate:
					change.ExtraOperations = append(change.ExtraOperations, "recreate")
				case ResourceInstallTypeUpdate:
					change.ExtraOperations = append(change.ExtraOperations, "update")
				case ResourceInstallTypeApply:
					change.ExtraOperations = append(change.ExtraOperations, "blind apply")
				case ResourceInstallTypeNone:
				default:
					panic("unexpected resource must install condition")
				}

				if info.MustDeleteOnSuccessfulInstall {
					change.ExtraOperations = append(change.ExtraOperations, "delete")
				}
			}
		}
	}

	return changes, nil
}

func buildDelChanges(delInfos []*DeletableResourceInfo, opts CalculatePlannedChangesOptions) ([]*ResourceChange, error) {
	var changes []*ResourceChange
	for _, info := range delInfos {
		if !info.MustDelete {
			continue
		}

		change, err := buildResourceChange(info.ResourceMeta, info.GetResult, nil, false, "delete", color.Style{color.Bold, color.Red}, opts)
		if err != nil {
			return nil, fmt.Errorf("build resource change for delete: %w", err)
		}

		changes = append(changes, change)
	}

	return changes, nil
}

func buildResourceChange(resMeta *spec.ResourceMeta, oldUnstruct, newUnstruct *unstructured.Unstructured, deleteAfter bool, opType string, opTypeStyle color.Style, opts CalculatePlannedChangesOptions) (*ResourceChange, error) {
	sensitiveInfo := resource.GetSensitiveInfo(resMeta.GroupVersionKind.GroupKind(), resMeta.Annotations)

	var uDiff string
	if spec.IsCRD(resMeta.GroupVersionKind.GroupKind()) &&
		!opts.ShowVerboseCRDDiffs &&
		(oldUnstruct == nil || newUnstruct == nil) {
		uDiff = HiddenVerboseCRDChanges
	} else if sensitiveInfo.FullySensitive() && !opts.ShowSensitiveDiffs {
		uDiff = HiddenSensitiveChanges
	} else if !opts.ShowVerboseDiffs && (oldUnstruct == nil || newUnstruct == nil) {
		uDiff = HiddenVerboseChanges
	} else {
		var (
			oldObjManifest string
			newObjManifest string
		)

		if oldUnstruct != nil {
			oldUnstructClean := cleanUnstruct(oldUnstruct, sensitiveInfo, opts)

			if oldObjByte, err := yaml.MarshalContext(context.TODO(), oldUnstructClean.Object, yaml.UseLiteralStyleIfMultiline(true)); err != nil {
				return nil, fmt.Errorf("marshal old unstruct to yaml: %w", err)
			} else {
				oldObjManifest = string(oldObjByte)
			}
		}

		if newUnstruct != nil {
			newUnstructClean := cleanUnstruct(newUnstruct, sensitiveInfo, opts)

			if newObjByte, err := yaml.MarshalContext(context.TODO(), newUnstructClean.Object, yaml.UseLiteralStyleIfMultiline(true)); err != nil {
				return nil, fmt.Errorf("marshal new unstruct to yaml: %w", err)
			} else {
				newObjManifest = string(newObjByte)
			}
		}

		uDiff = util.ColoredUnifiedDiff(oldObjManifest, newObjManifest, opts.DiffContextLines)
	}

	if uDiff == "" {
		uDiff = HiddenInsignificantChanges
	}

	var extraOps []string
	if deleteAfter {
		extraOps = append(extraOps, "delete")
	}

	return &ResourceChange{
		ResourceMeta:    resMeta,
		Type:            opType,
		TypeStyle:       opTypeStyle,
		Udiff:           uDiff,
		ExtraOperations: extraOps,
	}, nil
}

func cleanUnstruct(unstruct *unstructured.Unstructured, sensitiveInfo resource.SensitiveInfo, opts CalculatePlannedChangesOptions) *unstructured.Unstructured {
	var unstructClean *unstructured.Unstructured
	if sensitiveInfo.IsSensitive && !opts.ShowSensitiveDiffs {
		unstructClean = resource.RedactSensitiveData(unstruct, sensitiveInfo.SensitivePaths)
	} else {
		unstructClean = unstruct
	}

	cleanUnstructOpts := spec.CleanUnstructOptions{
		CleanRuntimeData: true,
	}

	if !opts.ShowInsignificantDiffs {
		cleanUnstructOpts.CleanHelmShAnnos = true
		cleanUnstructOpts.CleanWerfIoAnnos = true
		cleanUnstructOpts.CleanManagedFields = true
	}

	unstructClean = spec.CleanUnstruct(unstructClean, cleanUnstructOpts)

	return unstructClean
}
