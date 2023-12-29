package reprt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/release"
	"nelm.sh/nelm/pkg/log"
	"nelm.sh/nelm/pkg/opertn"
	"nelm.sh/nelm/pkg/rls"
	"nelm.sh/nelm/pkg/utls"
)

func NewReport(completedOps, canceledOps, failedOps []opertn.Operation, release *rls.Release) *Report {
	sort.Slice(completedOps, func(i, j int) bool {
		return completedOps[i].HumanID() < completedOps[j].HumanID()
	})
	sort.Slice(canceledOps, func(i, j int) bool {
		return canceledOps[i].HumanID() < canceledOps[j].HumanID()
	})
	sort.Slice(failedOps, func(i, j int) bool {
		return failedOps[i].HumanID() < failedOps[j].HumanID()
	})

	return &Report{
		completedOps: completedOps,
		failedOps:    failedOps,
		canceledOps:  canceledOps,
		release:      release,
	}
}

type Report struct {
	completedOps []opertn.Operation
	failedOps    []opertn.Operation
	canceledOps  []opertn.Operation
	release      *rls.Release
}

func (r *Report) Print(ctx context.Context) {
	totalOpsLen := len(r.completedOps) + len(r.failedOps) + len(r.canceledOps)
	if totalOpsLen == 0 {
		return
	}

	if len(r.completedOps) > 0 {
		log.Default.InfoBlock(ctx, completedStyle("Completed operations")).Do(func() {
			for _, op := range r.completedOps {
				log.Default.Info(ctx, utls.Capitalize(op.HumanID()))
			}
		})
	}

	if len(r.canceledOps) > 0 {
		log.Default.InfoBlock(ctx, canceledStyle("Canceled operations")).Do(func() {
			for _, op := range r.canceledOps {
				log.Default.Info(ctx, utls.Capitalize(op.HumanID()))
			}
		})
	}

	if len(r.failedOps) > 0 {
		log.Default.InfoBlock(ctx, failedStyle("Failed operations")).Do(func() {
			for _, op := range r.failedOps {
				log.Default.Info(ctx, utls.Capitalize(op.HumanID()))
			}
		})
	}

	log.Default.Info(ctx, color.Bold.Render("Operations summary:"))
	if len(r.completedOps) > 0 {
		log.Default.Info(ctx, "- "+completedStyle("completed:")+" %d operation(s)", len(r.completedOps))
	}
	if len(r.canceledOps) > 0 {
		log.Default.Info(ctx, "- "+canceledStyle("canceled:")+" %d operation(s)", len(r.canceledOps))
	}
	if len(r.failedOps) > 0 {
		log.Default.Info(ctx, "- "+failedStyle("failed:")+" %d operation(s)", len(r.failedOps))
	}
	log.Default.Info(ctx, "")
}

func (r *Report) JSON() ([]byte, error) {
	reportv2 := reportV2{
		Version:   2,
		Release:   r.release.Name(),
		Namespace: r.release.Namespace(),
		Revision:  r.release.Revision(),
		Status:    r.release.Status(),
		CompletedOperations: lo.Map(r.completedOps, func(op opertn.Operation, _ int) string {
			return op.ID()
		}),
		CanceledOperations: lo.Map(r.canceledOps, func(op opertn.Operation, _ int) string {
			return op.ID()
		}),
		FailedOperations: lo.Map(r.failedOps, func(op opertn.Operation, _ int) string {
			return op.ID()
		}),
	}

	data, err := json.MarshalIndent(reportv2, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("error marshalling report: %w", err)
	}

	return data, nil
}

func (r *Report) Save(path string) error {
	data, err := r.JSON()
	if err != nil {
		return fmt.Errorf("error constructing report JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
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
	Version             int            `json:"version,omitempty"`
	Release             string         `json:"release,omitempty"`
	Namespace           string         `json:"namespace,omitempty"`
	Revision            int            `json:"revision,omitempty"`
	Status              release.Status `json:"status,omitempty"`
	CompletedOperations []string       `json:"operations,omitempty"`
	CanceledOperations  []string       `json:"operations,omitempty"`
	FailedOperations    []string       `json:"operations,omitempty"`
}
