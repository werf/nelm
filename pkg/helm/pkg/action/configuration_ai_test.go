//go:build ai_tests

package action

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestAIConfigurationInitConcurrent(t *testing.T) {
	cfg := NewConfiguration()
	getter := genericclioptions.NewConfigFlags(true)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	for range 2 {
		go func() {
			defer wg.Done()
			errs <- cfg.Init(getter, "default", "memory")
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	require.NotNil(t, cfg.Releases)
}
