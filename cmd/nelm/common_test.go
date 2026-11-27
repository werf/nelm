//go:build unix

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
)

const cancelOnFirstSignalHelperEnv = "NELM_TEST_CANCEL_ON_FIRST_SIGNAL_HELPER"

func TestCancelOnFirstSignal_FirstSignalCancelsSecondExits(t *testing.T) {
	if os.Getenv(cancelOnFirstSignalHelperEnv) != "" {
		ctx := cancelOnFirstSignal(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)))

		fmt.Println("ready")
		<-ctx.Done()
		fmt.Println(context.Cause(ctx))

		select {}
	}

	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCancelOnFirstSignal_FirstSignalCancelsSecondExits$") //nolint:gosec // G204: re-runs this test binary as the helper process

			cmd.Env = append(os.Environ(), cancelOnFirstSignalHelperEnv+"=1")
			stdout, err := cmd.StdoutPipe()
			require.NoError(t, err)
			require.NoError(t, cmd.Start())

			lines := bufio.NewScanner(stdout)
			require.True(t, lines.Scan())
			require.Equal(t, "ready", lines.Text())

			require.NoError(t, cmd.Process.Signal(sig))
			require.True(t, lines.Scan())
			assert.Equal(t, fmt.Sprintf("context canceled: received signal %q", sig.String()), lines.Text())

			require.NoError(t, cmd.Process.Signal(sig))

			var exitErr *exec.ExitError
			require.ErrorAs(t, cmd.Wait(), &exitErr)
			assert.Equal(t, 128+int(sig), exitErr.ExitCode())
			require.NoError(t, ctx.Err(), "the second signal must exit the process")
		})
	}
}
