package action

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmcmd "github.com/werf/nelm/v2/pkg/helm/pkg/cmd"
	"github.com/werf/nelm/v2/pkg/log"
)

func TestSetupLoggingLogWidth(t *testing.T) {
	t.Run("explicit width overrides detected width", func(t *testing.T) {
		ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))

		ctx = SetupLogging(ctx, log.InfoLevel, SetupLoggingOptions{LogWidth: 213})

		assert.Equal(t, 213, log.Default.BlockContentWidth(ctx))
	})

	t.Run("zero width keeps detected width", func(t *testing.T) {
		ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
		detectedWidth := log.Default.BlockContentWidth(ctx)

		ctx = SetupLogging(ctx, log.InfoLevel, SetupLoggingOptions{})

		assert.Equal(t, detectedWidth, log.Default.BlockContentWidth(ctx))
	})
}

func TestSetupLoggingRoutesHelmForkMessagesThroughLogger(t *testing.T) {
	const (
		exportChart           = "./testdata/chart-render-log-messages/export-chart"
		invalidExportChart    = "./testdata/chart-render-log-messages/invalid-export-chart"
		exportOntoTableValues = "testdata/chart-render-log-messages/values-export-onto-table.yaml"
		coalesceWarning       = "warning: cannot overwrite table with non table for sub.cfg"
		exportDebugMessage    = "ExportValues parent path not found in chart values"
		invalidExportWarning  = "invalid ExportValues definition"
	)

	type output struct {
		errOut    string
		out       string
		rawStderr string
	}

	render := func(t *testing.T, chartPath string, level log.Level, valuesFiles []string) output {
		prevSlogDefault := slog.Default()
		t.Cleanup(func() { slog.SetDefault(prevSlogDefault) })

		stderrFile, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
		require.NoError(t, err)

		prevStderr := os.Stderr
		os.Stderr = stderrFile

		helmcmd.SetupLogging(false)

		os.Stderr = prevStderr

		var outBuf, errBuf bytes.Buffer

		ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&outBuf, &errBuf))
		ctx = SetupLogging(ctx, level, SetupLoggingOptions{})

		_, err = ChartRender(ctx, ChartRenderOptions{
			Chart:         chartPath,
			OutputNoPrint: true,
			ValuesOptions: common.ValuesOptions{ValuesFiles: valuesFiles},
		})
		require.NoError(t, err)
		require.NoError(t, stderrFile.Close())

		rawStderr, err := os.ReadFile(stderrFile.Name())
		require.NoError(t, err)

		return output{errOut: errBuf.String(), out: outBuf.String(), rawStderr: string(rawStderr)}
	}

	t.Run("coalesce warnings go to the error stream, not stdout, at debug level", func(t *testing.T) {
		got := render(t, exportChart, log.DebugLevel, []string{exportOntoTableValues})

		assert.Contains(t, got.errOut, coalesceWarning)
		assert.NotContains(t, got.out, coalesceWarning)
	})

	t.Run("coalesce warnings are shown at warning level", func(t *testing.T) {
		got := render(t, exportChart, log.WarningLevel, []string{exportOntoTableValues})

		assert.Contains(t, got.errOut, coalesceWarning)
	})

	t.Run("coalesce warnings are hidden at silent level", func(t *testing.T) {
		got := render(t, exportChart, log.SilentLevel, []string{exportOntoTableValues})

		assert.Equal(t, output{}, got)
	})

	t.Run("slog warnings are shown at warning level", func(t *testing.T) {
		got := render(t, invalidExportChart, log.WarningLevel, nil)

		assert.Contains(t, got.errOut, invalidExportWarning)
		assert.Empty(t, got.rawStderr)
	})

	t.Run("slog warnings are hidden at silent level", func(t *testing.T) {
		got := render(t, invalidExportChart, log.SilentLevel, nil)

		assert.Equal(t, output{}, got)
	})

	t.Run("slog warnings are hidden at error level", func(t *testing.T) {
		got := render(t, invalidExportChart, log.ErrorLevel, nil)

		assert.Equal(t, output{}, got)
	})

	t.Run("slog debug messages are shown at debug level", func(t *testing.T) {
		got := render(t, exportChart, log.DebugLevel, nil)

		assert.Contains(t, got.out, exportDebugMessage)
	})

	t.Run("slog debug messages are hidden at info level", func(t *testing.T) {
		got := render(t, exportChart, log.InfoLevel, nil)

		assert.NotContains(t, got.out+got.errOut+got.rawStderr, exportDebugMessage)
	})
}
