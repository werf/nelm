package plan

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/chanced/caps"
	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/meta"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

const (
	HiddenVerboseCRDChanges    = "<hidden verbose CRD changes>"
	HiddenVerboseChanges       = "<hidden verbose changes>"
	HiddenInsignificantChanges = "<hidden insignificant changes>"
	HiddenSensitiveChanges     = "<hidden sensitive changes>"
)

// TODO: expose to the user
type CalculatePlannedChangesOptions struct {
	DiffContextLines       int // FIXME(ilya-lesikov): don't forget 3 by default from upper levels
	ShowVerboseCRDDiffs    bool
	ShowVerboseDiffs       bool
	ShowSensitiveDiffs     bool
	ShowInsignificantDiffs bool
}

func CalculatePlannedChanges(installableInfos []*InstallableResourceInfo, deletableInfos []*DeletableResourceInfo, opts CalculatePlannedChangesOptions) ([]*ResourceChange, error) {
	instInfosByIter := groupInstInfosByIter(installableInfos)

	instChanges, err := buildInstChanges(instInfosByIter, opts)
	if err != nil {
		return nil, fmt.Errorf("build installable resource changes: %w", err)
	}

	sort.SliceStable(deletableInfos, func(i, j int) bool {
		return meta.ResourceMetaSortHandler(deletableInfos[i].ResourceMeta, deletableInfos[j].ResourceMeta)
	})

	delChanges, err := buildDelChanges(deletableInfos, opts)
	if err != nil {
		return nil, fmt.Errorf("build deletable resource changes: %w", err)
	}

	return append(instChanges, delChanges...), nil
}

func LogPlannedChanges(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	releaseChangesPlanned bool,
	changes []*ResourceChange,
) {
	if len(changes) == 0 {
		if releaseChangesPlanned {
			log.Default.Info(ctx, color.Style{color.Bold, color.Yellow}.Render(fmt.Sprintf("No resource changes planned, but will create release %q (namespace: %q)", releaseName, releaseNamespace)))
		} else {
			log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("No changes planned for release %q (namespace: %q)", releaseName, releaseNamespace)))
		}

		return
	}

	log.Default.Info(ctx, "")

	for _, change := range changes {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: buildDiffHeader(change),
		}, func() {
			log.Default.Info(ctx, "%s", change.Udiff)
		})
	}

	log.Default.Info(ctx, color.Bold.Render("Planned changes summary")+" for release %q (namespace: %q):", releaseName, releaseNamespace)
	for _, changeType := range []string{"create", "recreate", "update", "blind apply", "delete"} {
		logSummaryLine(ctx, changes, changeType)
	}
	log.Default.Info(ctx, "")
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
				default:
					panic("unexpected resource must install condition")
				}

				changes = append(changes, change)
			}
		} else {
			for _, info := range instInfos {
				change := lo.Must(lo.Find(changes, func(change *ResourceChange) bool {
					return change.ResourceMeta.ID() == info.ID() && info.Iteration == iter-1
				}))

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

func buildResourceChange(resMeta *meta.ResourceMeta, oldUnstruct, newUnstruct *unstructured.Unstructured, deleteAfter bool, opType string, opTypeStyle color.Style, opts CalculatePlannedChangesOptions) (*ResourceChange, error) {
	sensitiveInfo := resource.GetSensitiveInfo(resMeta.GroupVersionKind.GroupKind(), resMeta.Annotations)

	var uDiff string
	if resource.IsCRD(resMeta.GroupVersionKind.GroupKind()) &&
		!opts.ShowVerboseCRDDiffs &&
		(oldUnstruct == nil || newUnstruct == nil) {
		uDiff = HiddenVerboseCRDChanges
	} else if sensitiveInfo.FullySensitive() && !opts.ShowSensitiveDiffs {
		uDiff = HiddenSensitiveChanges
	} else if !opts.ShowVerboseDiffs && (oldUnstruct == nil || newUnstruct == nil) {
		uDiff = HiddenVerboseChanges
	} else {
		var oldObjManifest string
		var newObjManifest string
		if oldUnstruct != nil {
			oldUnstructClean := cleanUnstruct(oldUnstruct, sensitiveInfo, opts)

			if oldObjByte, err := oldUnstructClean.MarshalJSON(); err != nil {
				return nil, fmt.Errorf("marshal old unstruct to json: %w", err)
			} else {
				oldObjManifest = string(oldObjByte)
			}
		}

		if newUnstruct != nil {
			newUnstructClean := cleanUnstruct(newUnstruct, sensitiveInfo, opts)

			if newObjByte, err := newUnstructClean.MarshalJSON(); err != nil {
				return nil, fmt.Errorf("marshal new unstruct to json: %w", err)
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

	cleanUnstructOpts := resource.CleanUnstructOptions{
		CleanRuntimeData: true,
	}

	if !opts.ShowInsignificantDiffs {
		cleanUnstructOpts.CleanHelmShAnnos = true
		cleanUnstructOpts.CleanWerfIoAnnos = true
		cleanUnstructOpts.CleanManagedFiles = true
	}

	unstructClean = resource.CleanUnstruct(unstructClean, cleanUnstructOpts)

	return unstructClean
}

func buildDiffHeader(change *ResourceChange) string {
	header := change.TypeStyle.Render(caps.ToUpper(change.Type))
	header += " " + color.Style{color.Bold}.Render(change.ResourceMeta.IDHuman())

	var headerOps []string
	for _, op := range change.ExtraOperations {
		headerOps = append(headerOps, color.Style{color.Bold}.Render(op))
	}

	if len(headerOps) > 0 {
		header += ", then " + strings.Join(headerOps, ", ")
	}

	if change.Reason != "" {
		header += ". Reason: " + change.Reason
	}

	return header
}

func logSummaryLine(ctx context.Context, changes []*ResourceChange, changeType string) {
	filteredChanges := lo.Filter(changes, func(change *ResourceChange, _ int) bool {
		return change.Type == changeType
	})

	if len(filteredChanges) > 0 {
		log.Default.Info(ctx, "- %s: %d resources", filteredChanges[0].TypeStyle.Render("create"), len(filteredChanges))
	}
}

type ResourceChange struct {
	ExtraOperations []string
	Reason          string
	ResourceMeta    *meta.ResourceMeta
	Type            string
	TypeStyle       color.Style
	Udiff           string
}
