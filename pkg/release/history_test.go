package release

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
)

var _ ReleaseStorager = (*failingDeleteStorage)(nil)

type failingDeleteStorage struct {
	deleteErr error
}

func (s *failingDeleteStorage) Create(rls *helmrelease.Release) error {
	return nil
}

func (s *failingDeleteStorage) Delete(name string, version int) (*helmrelease.Release, error) {
	return nil, s.deleteErr
}

func (s *failingDeleteStorage) GetRelease(name string, version int) (*helmrelease.Release, error) {
	return nil, errors.ErrUnsupported
}

func (s *failingDeleteStorage) Query(labels map[string]string) ([]*helmrelease.Release, error) {
	return nil, nil
}

func (s *failingDeleteStorage) Update(rls *helmrelease.Release) error {
	return nil
}

func TestHistoryDeleteRelease_StorageError(t *testing.T) {
	storage := &failingDeleteStorage{deleteErr: errors.New("context deadline exceeded")}
	history := NewHistory(nil, "myrel", storage, HistoryOptions{})

	var err error
	require.NotPanics(t, func() {
		err = history.DeleteRelease(context.Background(), "myrel", 7)
	})

	require.Error(t, err)
	require.ErrorIs(t, err, storage.deleteErr)
	assert.Contains(t, err.Error(), "myrel")
	assert.Contains(t, err.Error(), "7")
}
