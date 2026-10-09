package action

import (
	"context"
	"fmt"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/nelm/v2/pkg/release"
)

type latestReleaseListLoads struct {
	errs      []error
	revisions []release.Revision
}

type latestReleaseListStorager struct {
	release.ReleaseStorager

	err              error
	historyLimit     int
	historyName      string
	historySummaries []release.RevisionSummary
	latestSummaries  []release.RevisionSummary
	// loadGate, when set, holds every LoadRevisionSummary call until it is closed or the
	// call's context ends; loadStarted then receives one value per call.
	loadGate    chan struct{}
	loadStarted chan struct{}
	loads       *kdutil.Concurrent[*latestReleaseListLoads]
	revisions   []release.Revision
	selector    string
	summaries   map[string]*release.ReleaseSummary
}

func (s *latestReleaseListStorager) LatestRevisions(ctx context.Context, opts release.LatestRevisionsOptions) ([]release.Revision, error) {
	s.selector = opts.LabelSelector
	if s.err != nil {
		return nil, s.err
	}

	return s.revisions, nil
}

func (s *latestReleaseListStorager) ListLatestSummaries(ctx context.Context, opts release.ListLatestSummariesOptions) ([]release.RevisionSummary, error) {
	s.selector = opts.LabelSelector
	if s.err != nil {
		return nil, s.err
	}

	return s.latestSummaries, nil
}

func (s *latestReleaseListStorager) ListRevisionSummaries(ctx context.Context, name string, opts release.ListRevisionSummariesOptions) ([]release.RevisionSummary, error) {
	s.historyName, s.historyLimit = name, opts.Limit
	if s.err != nil {
		return nil, s.err
	}

	return s.historySummaries, nil
}

func (s *latestReleaseListStorager) LoadRevisionSummary(ctx context.Context, revision release.Revision) (*release.ReleaseSummary, error) {
	if s.loadGate != nil {
		s.loadStarted <- struct{}{}

		select {
		case <-s.loadGate:
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for load gate: %w", ctx.Err())
		}
	}

	var err error

	s.loads.RWTransaction(func(loads *latestReleaseListLoads) {
		loads.revisions = append(loads.revisions, revision)
		if len(loads.errs) > 0 {
			err = loads.errs[0]
			loads.errs = loads.errs[1:]
		}
	})

	if err != nil {
		return nil, err
	}

	return s.summaries[revision.Namespace+"/"+revision.Name], nil
}

func (s *latestReleaseListStorager) loadedRevisions() []release.Revision {
	if s.loads == nil {
		return nil
	}

	var revisions []release.Revision

	s.loads.RTransaction(func(loads *latestReleaseListLoads) {
		revisions = append(revisions, loads.revisions...)
	})

	return revisions
}

func newLatestRevisionSummary(namespace, name string, version int) release.RevisionSummary {
	return release.RevisionSummary{
		Revision: release.Revision{Namespace: namespace, Name: name, Version: version, Status: "deployed"},
		Summary:  newTestReleaseSummary(),
	}
}

func newMetadataReleaseListStorager(revisions []release.Revision, summaries map[string]*release.ReleaseSummary, loadErrs ...error) *latestReleaseListStorager {
	return &latestReleaseListStorager{
		loads:     kdutil.NewConcurrent(&latestReleaseListLoads{errs: loadErrs}),
		revisions: revisions,
		summaries: summaries,
	}
}

func newTestReleaseSummary() *release.ReleaseSummary {
	return &release.ReleaseSummary{
		Annotations: map[string]string{"managed-by": "deckhouse"},
		Chart:       &release.ReleaseSummaryChart{AppVersion: "4.5.6", Name: "mychart", Version: "1.2.3"},
	}
}
