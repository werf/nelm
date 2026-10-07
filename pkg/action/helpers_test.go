package action

import (
	"context"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/nelm/v2/pkg/release"
)

type latestReleaseListEntry struct {
	err      error
	revision release.Revision
	summary  *release.ReleaseSummary
}

func newLatestReleaseListEntry(namespace, name string, version int) latestReleaseListEntry {
	return latestReleaseListEntry{
		revision: release.Revision{Namespace: namespace, Name: name, Version: version, Status: "deployed"},
		summary:  newTestReleaseSummary(),
	}
}

type latestReleaseListLoads struct {
	errs      []error
	revisions []release.Revision
}

type latestReleaseListStorager struct {
	release.ReleaseStorager

	entries   []latestReleaseListEntry
	err       error
	loads     *kdutil.Concurrent[*latestReleaseListLoads]
	revisions []release.Revision
	selector  string
	summaries map[string]*release.ReleaseSummary
}

func (s *latestReleaseListStorager) ForEachLatestRelease(ctx context.Context, fn func(release.Revision, *release.ReleaseSummary, error) error, opts release.ForEachLatestReleaseOptions) error {
	s.selector = opts.LabelSelector
	if s.err != nil {
		return s.err
	}

	for _, entry := range s.entries {
		if err := fn(entry.revision, entry.summary, entry.err); err != nil {
			return err
		}
	}

	return nil
}

func (s *latestReleaseListStorager) LatestRevisions(ctx context.Context, opts release.LatestRevisionsOptions) ([]release.Revision, error) {
	s.selector = opts.LabelSelector
	if s.err != nil {
		return nil, s.err
	}

	return s.revisions, nil
}

func (s *latestReleaseListStorager) LoadRevisionSummary(ctx context.Context, revision release.Revision) (*release.ReleaseSummary, error) {
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
