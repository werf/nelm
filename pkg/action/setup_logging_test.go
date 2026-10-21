package action

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/logboek"
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
