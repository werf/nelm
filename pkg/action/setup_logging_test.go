package action

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
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
		chartPath             = "./testdata/chart-render-log-messages/chart"
		exportOntoTableValues = "testdata/chart-render-log-messages/values-export-onto-table.yaml"
		coalesceWarning       = "warning: cannot overwrite table with non table for sub.cfg"
		exportDebugMessage    = "ExportValues parent path not found in chart values"
	)

	render := func(t *testing.T, level log.Level, valuesFiles []string) (string, string) {
		prevSlogDefault := slog.Default()
		t.Cleanup(func() { slog.SetDefault(prevSlogDefault) })

		var outBuf, errBuf bytes.Buffer

		ctx := logboek.NewContext(context.Background(), logboek.NewLogger(&outBuf, &errBuf))
		ctx = SetupLogging(ctx, level, SetupLoggingOptions{})

		_, err := ChartRender(ctx, ChartRenderOptions{
			Chart:         chartPath,
			OutputNoPrint: true,
			ValuesOptions: common.ValuesOptions{ValuesFiles: valuesFiles},
		})
		require.NoError(t, err)

		return outBuf.String(), errBuf.String()
	}

	t.Run("coalesce warnings go to the error stream, not stdout, at debug level", func(t *testing.T) {
		out, errOut := render(t, log.DebugLevel, []string{exportOntoTableValues})

		assert.Contains(t, errOut, coalesceWarning)
		assert.NotContains(t, out, coalesceWarning)
	})

	t.Run("coalesce warnings are shown at warning level", func(t *testing.T) {
		_, errOut := render(t, log.WarningLevel, []string{exportOntoTableValues})

		assert.Contains(t, errOut, coalesceWarning)
	})

	t.Run("nothing is printed at silent level", func(t *testing.T) {
		out, errOut := render(t, log.SilentLevel, []string{exportOntoTableValues})

		assert.Empty(t, out)
		assert.Empty(t, errOut)
	})

	t.Run("slog debug messages are shown at debug level", func(t *testing.T) {
		out, _ := render(t, log.DebugLevel, nil)

		assert.Contains(t, out, exportDebugMessage)
	})

	t.Run("slog debug messages are hidden at info level", func(t *testing.T) {
		out, errOut := render(t, log.InfoLevel, nil)

		assert.NotContains(t, out+errOut, exportDebugMessage)
	})
}
