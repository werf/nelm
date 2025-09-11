package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/chanced/caps"
	"github.com/gookit/color"
	"github.com/samber/lo"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

func newReport(completedOps, canceledOps, failedOps []operation.FixmeOperation, release *release.Release) *report {
	sort.Slice(completedOps, func(i, j int) bool {
		return completedOps[i].IDHuman() < completedOps[j].IDHuman()
	})
	sort.Slice(canceledOps, func(i, j int) bool {
		return canceledOps[i].IDHuman() < canceledOps[j].IDHuman()
	})
	sort.Slice(failedOps, func(i, j int) bool {
		return failedOps[i].IDHuman() < failedOps[j].IDHuman()
	})

	return &report{
		completedOps: completedOps,
		failedOps:    failedOps,
		canceledOps:  canceledOps,
		release:      release,
	}
}

type report struct {
	completedOps []operation.FixmeOperation
	failedOps    []operation.FixmeOperation
	canceledOps  []operation.FixmeOperation
	release      *release.Release
}

func (r *report) Print(ctx context.Context) {
	totalOpsLen := len(r.completedOps) + len(r.failedOps) + len(r.canceledOps)
	if totalOpsLen == 0 {
		return
	}

	if len(r.completedOps) > 0 {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: completedStyle("Completed operations"),
		}, func() {
			for _, op := range r.completedOps {
				log.Default.Info(ctx, caps.ToUpper(op.IDHuman()))
			}
		})
	}

	if len(r.canceledOps) > 0 {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: canceledStyle("Canceled operations"),
		}, func() {
			for _, op := range r.canceledOps {
				log.Default.Info(ctx, caps.ToUpper(op.IDHuman()))
			}
		})
	}

	if len(r.failedOps) > 0 {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: failedStyle("Failed operations"),
		}, func() {
			for _, op := range r.failedOps {
				log.Default.Info(ctx, caps.ToUpper(op.IDHuman()))
			}
		})
	}
}

func (r *report) JSON() ([]byte, error) {
	reportv2 := reportV2{
		Version:   2,
		Release:   r.release.Name(),
		Namespace: r.release.Namespace(),
		Revision:  r.release.Revision(),
		Status:    r.release.Status(),
		CompletedOperations: lo.Map(r.completedOps, func(op operation.FixmeOperation, _ int) string {
			return op.ID()
		}),
		CanceledOperations: lo.Map(r.canceledOps, func(op operation.FixmeOperation, _ int) string {
			return op.ID()
		}),
		FailedOperations: lo.Map(r.failedOps, func(op operation.FixmeOperation, _ int) string {
			return op.ID()
		}),
	}

	data, err := json.MarshalIndent(reportv2, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("error marshaling report: %w", err)
	}

	return data, nil
}

func (r *report) Save(path string) error {
	data, err := r.JSON()
	if err != nil {
		return fmt.Errorf("error constructing report JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("error writing report file at %q: %w", path, err)
	}

	return nil
}

func completedStyle(text string) string {
	return color.Style{color.Bold, color.Green}.Render(text)
}

func canceledStyle(text string) string {
	return color.Style{color.Bold, color.Yellow}.Render(text)
}

func failedStyle(text string) string {
	return color.Style{color.Bold, color.Red}.Render(text)
}

type reportV2 struct {
	Version             int                `json:"version,omitempty"`
	Release             string             `json:"release,omitempty"`
	Namespace           string             `json:"namespace,omitempty"`
	Revision            int                `json:"revision,omitempty"`
	Status              helmrelease.Status `json:"status,omitempty"`
	CompletedOperations []string           `json:"operations,omitempty"`
	CanceledOperations  []string           `json:"operations,omitempty"`
	FailedOperations    []string           `json:"operations,omitempty"`
}
