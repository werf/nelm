package action

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	chartv2 "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/v2/pkg/release"
)

type latestReleaseListEntry struct {
	err      error
	rel      helmrel.Accessor
	revision release.Revision
}

func newLatestReleaseListEntry(t *testing.T, namespace, name string, version int) latestReleaseListEntry {
	t.Helper()

	acc, err := helmrel.NewAccessor(&helmrelease.Release{
		Name: name, Namespace: namespace, Version: version,
		Info:  &helmrelease.Info{Status: helmreleasestatus.StatusDeployed, Annotations: map[string]string{"managed-by": "deckhouse"}},
		Chart: &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "mychart", Version: "1.2.3", AppVersion: "4.5.6"}},
	})
	require.NoError(t, err)

	return latestReleaseListEntry{
		rel:      acc,
		revision: release.Revision{Namespace: namespace, Name: name, Version: version, Status: "deployed"},
	}
}

type latestReleaseListStorager struct {
	release.ReleaseStorager

	entries  []latestReleaseListEntry
	err      error
	selector string
}

func (s *latestReleaseListStorager) ForEachLatestRelease(ctx context.Context, fn func(release.Revision, helmrel.Accessor, error) error, opts release.ForEachLatestReleaseOptions) error {
	s.selector = opts.LabelSelector
	if s.err != nil {
		return s.err
	}

	for _, entry := range s.entries {
		if err := fn(entry.revision, entry.rel, entry.err); err != nil {
			return err
		}
	}

	return nil
}
