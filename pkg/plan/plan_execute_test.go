package plan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReadinessTrackerOptionsIgnoreLogs(t *testing.T) {
	for _, tt := range []struct {
		name             string
		opConfigIgnore   bool
		noPodLogs        bool
		expectIgnoreLogs bool
	}{
		{name: "logs enabled", opConfigIgnore: false, noPodLogs: false, expectIgnoreLogs: false},
		{name: "no-pod-logs at execution", opConfigIgnore: false, noPodLogs: true, expectIgnoreLogs: true},
		{name: "ignored in plan", opConfigIgnore: true, noPodLogs: false, expectIgnoreLogs: true},
		{name: "ignored in plan and at execution", opConfigIgnore: true, noPodLogs: true, expectIgnoreLogs: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opConfig := &OperationConfigTrackReadiness{
				IgnoreLogs:              tt.opConfigIgnore,
				IgnoreLogsForContainers: []string{"sidecar"},
				SaveEvents:              true,
			}

			opts := readinessTrackerOptions(opConfig, time.Minute, tt.noPodLogs)

			assert.Equal(t, tt.expectIgnoreLogs, opts.IgnoreLogs)
			assert.Equal(t, tt.opConfigIgnore, opConfig.IgnoreLogs)
			assert.Equal(t, time.Minute, opts.Timeout)
			assert.Equal(t, []string{"sidecar"}, opts.IgnoreLogsForContainers)
			assert.True(t, opts.SaveEvents)
		})
	}
}
