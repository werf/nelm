package plan

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/pkg/legacy/progrep"
)

type LegacyProgressReporter struct {
	state    *kdutil.Concurrent[*progressReporterState]
	reportCh chan<- progrep.ProgressReport
}

type progressReporterState struct {
	frozen  []progrep.StageReport
	ops     []opEntry
	opIndex map[string]int
}

type opEntry struct {
	ref         progrep.ObjectRef
	typ         progrep.OperationType
	status      progrep.OperationStatus
	predIndices []int
}

func NewLegacyProgressReporter(reportCh chan<- progrep.ProgressReport) *LegacyProgressReporter {
	if cap(reportCh) < 1 {
		panic(fmt.Sprintf("LegacyProgressReportCh must be a buffered channel with capacity >= 1, got capacity %d", cap(reportCh)))
	}

	return &LegacyProgressReporter{
		state: kdutil.NewConcurrent(&progressReporterState{
			opIndex: make(map[string]int),
		}),
		reportCh: reportCh,
	}
}

func (r *LegacyProgressReporter) ReportStatus(opID string, status progrep.OperationStatus) {
	r.state.RWTransaction(func(s *progressReporterState) {
		idx, ok := s.opIndex[opID]
		if !ok {
			return
		}

		s.ops[idx].status = status

		report := buildProgressReport(s.frozen, s.ops)
		sendNonBlocking(r.reportCh, report)
	})
}

func (r *LegacyProgressReporter) Stop(ctx context.Context) {
	var report progrep.ProgressReport

	r.state.RWTransaction(func(s *progressReporterState) {
		report = buildProgressReport(s.frozen, s.ops)
	})

	select {
	case r.reportCh <- report:
	case <-ctx.Done():
	}
}

func (r *LegacyProgressReporter) startStage(p *Plan, resolvedNamespaces map[string]string) {
	r.state.RWTransaction(func(s *progressReporterState) {
		if len(s.ops) > 0 {
			s.frozen = append(s.frozen, buildStageReport(s.ops))
		}

		predMap := lo.Must(p.Graph.PredecessorMap())
		ops := p.Operations()

		var entries []opEntry

		entryIndex := make(map[string]int)

		for _, op := range ops {
			if op.Category != OperationCategoryResource && op.Category != OperationCategoryTrack {
				continue
			}

			ref := extractObjectRef(op, resolvedNamespaces)
			typ := mapOperationType(op.Type)
			idx := len(entries)
			entryIndex[op.ID()] = idx

			entries = append(entries, opEntry{
				ref:    ref,
				typ:    typ,
				status: progrep.OperationStatusPending,
			})
		}

		for _, op := range ops {
			idx, ok := entryIndex[op.ID()]
			if !ok {
				continue
			}

			var predIndices []int
			for predID := range predMap[op.ID()] {
				if predIdx, predOk := entryIndex[predID]; predOk {
					predIndices = append(predIndices, predIdx)
				}
			}

			entries[idx].predIndices = predIndices
		}

		s.ops = entries
		s.opIndex = entryIndex

		report := buildProgressReport(s.frozen, s.ops)
		sendNonBlocking(r.reportCh, report)
	})
}

func buildStageReport(ops []opEntry) progrep.StageReport {
	operations := make([]progrep.Operation, len(ops))
	for i, e := range ops {
		operations[i] = progrep.Operation{
			ObjectRef: e.ref,
			Type:      e.typ,
			Status:    e.status,
		}
	}

	return progrep.StageReport{
		Operations: operations,
	}
}

func buildProgressReport(frozen []progrep.StageReport, ops []opEntry) progrep.ProgressReport {
	stageReports := make([]progrep.StageReport, 0, len(frozen)+1)

	for _, sr := range frozen {
		opsCopy := make([]progrep.Operation, len(sr.Operations))
		copy(opsCopy, sr.Operations)
		stageReports = append(stageReports, progrep.StageReport{Operations: opsCopy})
	}

	operations := make([]progrep.Operation, len(ops))
	for i, e := range ops {
		var waitingFor []progrep.ObjectRef

		for _, predIdx := range e.predIndices {
			if ops[predIdx].status != progrep.OperationStatusCompleted {
				waitingFor = append(waitingFor, ops[predIdx].ref)
			}
		}

		operations[i] = progrep.Operation{
			ObjectRef:  e.ref,
			Type:       e.typ,
			Status:     e.status,
			WaitingFor: waitingFor,
		}
	}

	stageReports = append(stageReports, progrep.StageReport{Operations: operations})

	return progrep.ProgressReport{
		StageReports: stageReports,
	}
}

func sendNonBlocking(ch chan<- progrep.ProgressReport, report progrep.ProgressReport) {
	select {
	case ch <- report:
	default:
	}
}
