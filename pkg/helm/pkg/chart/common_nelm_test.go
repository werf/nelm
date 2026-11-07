package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
)

func TestV3Accessor_MetaDependencies_Unvendored(t *testing.T) {
	dep := &v3chart.Dependency{Name: "dep", Version: "1.0.0", Repository: "https://example.com"}
	accessor, err := NewAccessor(&v3chart.Chart{Metadata: &v3chart.Metadata{Name: "mychart", Dependencies: []*v3chart.Dependency{dep}}})
	require.NoError(t, err)

	assert.Equal(t, []Dependency{dep}, accessor.MetaDependencies())
}
