//go:build ai_tests

package action

import (
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestAIConfigurationInitAndSetHookOutputFuncConcurrent(t *testing.T) {
	cfg := NewConfiguration()
	getter := genericclioptions.NewConfigFlags(true)
	start := make(chan struct{})
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start
		errs <- cfg.Init(getter, "default", "memory")
	}()
	go func() {
		defer wg.Done()
		<-start
		cfg.SetHookOutputFunc(func(_, _, _ string) io.Writer { return io.Discard })
	}()

	close(start)
	wg.Wait()

	require.NoError(t, <-errs)
	require.NotNil(t, cfg.Releases)
	require.NotNil(t, cfg.HookOutputFunc)
}
