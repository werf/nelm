package reprt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/opertn"
	"helm.sh/helm/v3/pkg/werf/rls"
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
	if len(r.completedOps) > 0 {
		log.Default.Info(ctx, "Completed operations:")
		for _, op := range r.completedOps {
			log.Default.Info(ctx, "- %s", op.HumanID())
		}
	}

	if len(r.canceledOps) > 0 {
		log.Default.Info(ctx, "Canceled operations:")
		for _, op := range r.canceledOps {
			log.Default.Info(ctx, "- %s", op.HumanID())
		}
	}

	if len(r.failedOps) > 0 {
		log.Default.Info(ctx, "Failed operations:")
		for _, op := range r.failedOps {
			log.Default.Info(ctx, "- %s", op.HumanID())
		}
	}
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
