//go:build ai_tests

package track

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/werf/kubedog/pkg/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/logboek"
)

func TestAIProgressTablesPrinterStartInitializesBeforeReturn(t *testing.T) {
	printer := NewProgressTablesPrinter(
		kdutil.NewConcurrent(statestore.NewTaskStore()),
		kdutil.NewConcurrent(logstore.NewLogStore()),
		ProgressTablesPrinterOptions{},
	)

	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
	printer.Start(ctx, time.Hour)
	require.NotNil(t, printer.ctxCancelFn)
	require.NotNil(t, printer.finishedCh)

	printer.Stop()
	printer.Wait()
}
