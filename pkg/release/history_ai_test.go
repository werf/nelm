//go:build ai_tests

package release

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
)

var _ ReleaseStorager = (*stubStorager)(nil)

type stubStorager struct {
	deleteErr error
	deleteRel helmrel.Accessor
}

func (s *stubStorager) Create(rls helmrel.Accessor) error {
	return nil
}

func (s *stubStorager) Delete(name string, version int) (helmrel.Accessor, error) {
	return s.deleteRel, s.deleteErr
}

func (s *stubStorager) GetRelease(name string, version int) (helmrel.Accessor, error) {
	return nil, nil
}

func (s *stubStorager) ListLatestReleases(ctx context.Context) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *stubStorager) Query(labels map[string]string) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *stubStorager) Revisions(ctx context.Context, name string) ([]Revision, error) {
	return nil, nil
}

func (s *stubStorager) Update(rls helmrel.Accessor) error {
	return nil
}

func (s *stubStorager) UpdateLabels(name string, version int, labels map[string]string) error {
	return nil
}

func TestAI_DeleteRelease_ErrorIncludesNameAndRevision(t *testing.T) {
	storage := &stubStorager{
		deleteErr: errors.New("kube delete failed"),
		deleteRel: nil,
	}
	history := NewHistory(nil, "myrelease", storage, HistoryOptions{})

	var delErr error
	require.NotPanics(t, func() {
		delErr = history.DeleteRelease(context.Background(), "myrelease", 3)
	})

	require.Error(t, delErr)
	assert.ErrorIs(t, delErr, storage.deleteErr)
	assert.Contains(t, delErr.Error(), `"myrelease"`)
	assert.Contains(t, delErr.Error(), "revision: 3")
}
