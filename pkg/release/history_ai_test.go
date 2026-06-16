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

func (s *stubStorager) Query(labels map[string]string) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *stubStorager) Update(rls helmrel.Accessor) error {
	return nil
}

func TestAI_DeleteRelease_ErrorIncludesNameAndRevision(t *testing.T) {
	history := NewHistory(nil, "myrelease", &stubStorager{
		deleteErr: errors.New("kube delete failed"),
		deleteRel: nil,
	}, HistoryOptions{})

	delErr := history.DeleteRelease(context.Background(), "myrelease", 3)
	require.Error(t, delErr)
	assert.Contains(t, delErr.Error(), `"myrelease"`)
	assert.Contains(t, delErr.Error(), "revision: 3")
}
