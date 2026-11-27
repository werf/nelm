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

func TestCancelOnFirstSignal_FirstSignalCancelsSecondTerminates(t *testing.T) {
	if os.Getenv(cancelOnFirstSignalHelperEnv) != "" {
		ctx := cancelOnFirstSignal(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)))

		fmt.Println("ready")
		<-ctx.Done()
		fmt.Println(context.Cause(ctx))

		select {}
	}

	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestCancelOnFirstSignal_FirstSignalCancelsSecondTerminates$") //nolint:gosec // G204: re-runs this test binary as the helper process

			cmd.Env = append(os.Environ(), cancelOnFirstSignalHelperEnv+"=1")
			stdout, err := cmd.StdoutPipe()
			require.NoError(t, err)
			require.NoError(t, cmd.Start())

			t.Cleanup(func() {
				_ = cmd.Process.Kill()
			})

			lines := bufio.NewScanner(stdout)
			require.True(t, lines.Scan())
			require.Equal(t, "ready", lines.Text())

			require.NoError(t, cmd.Process.Signal(sig))
			require.True(t, lines.Scan())
			assert.Equal(t, fmt.Sprintf("context canceled: received signal %q", sig.String()), lines.Text())

			require.NoError(t, cmd.Process.Signal(sig))

			waitErrCh := make(chan error, 1)
			go func() {
				waitErrCh <- cmd.Wait()
			}()

			select {
			case err := <-waitErrCh:
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)

				status, ok := exitErr.Sys().(syscall.WaitStatus)
				require.True(t, ok)
				assert.True(t, status.Signaled())
				assert.Equal(t, sig, status.Signal())
			case <-time.After(10 * time.Second):
				t.Fatal("the second signal must terminate the process")
			}
		})
	}
}
