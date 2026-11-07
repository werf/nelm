package ts_test

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/ts"
)

func TestBundleChartsRecursive_ReuseDoesNotNeedDeno(t *testing.T) {
	accessor, err := helmchart.NewAccessor(&v2chart.Chart{
		Metadata: &v2chart.Metadata{APIVersion: v2chart.APIVersionV2, Name: "mychart", Version: "1.0.0"},
		RuntimeFiles: []*chartcommon.File{
			{Name: common.ChartTSSourceDir + common.ChartTSEntryPointTS, Data: []byte("export {};\n")},
			{Name: common.ChartTSBundleFile, Data: []byte("bundle\n")},
		},
	})
	require.NoError(t, err)

	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
	missingDeno := filepath.Join(t.TempDir(), "deno")
	require.NoError(t, ts.BundleChartsRecursive(ctx, accessor, t.TempDir(), false, missingDeno))

	require.ErrorContains(t, ts.BundleChartsRecursive(ctx, accessor, t.TempDir(), true, missingDeno), "deno binary not found")
}
