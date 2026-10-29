package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceKubeVersionPin(t *testing.T) {
	for _, tt := range []struct {
		errContains string
		name        string
		taskfile    string
		wantPinned  string
		wantTask    string
	}{
		{
			name:       "replaces the pin and keeps everything else",
			taskfile:   "vars:\n  version: \"0.0.0\"\n\n  kubeconformValidationSchemasUpstreamKubeVersion: \"1.36.3\"\n\n  tags: \"\"\n",
			wantPinned: "1.36.3",
			wantTask:   "vars:\n  version: \"0.0.0\"\n\n  kubeconformValidationSchemasUpstreamKubeVersion: \"1.37.1\"\n\n  tags: \"\"\n",
		},
		{
			name:       "leaves an up to date pin as is",
			taskfile:   "vars:\n  kubeconformValidationSchemasUpstreamKubeVersion: \"1.37.1\"\n",
			wantPinned: "1.37.1",
			wantTask:   "vars:\n  kubeconformValidationSchemasUpstreamKubeVersion: \"1.37.1\"\n",
		},
		{
			errContains: "found 0",
			name:        "fails without a pin",
			taskfile:    "vars:\n  version: \"0.0.0\"\n",
		},
		{
			errContains: "found 0",
			name:        "ignores a reference to the pin",
			taskfile:    "    cmds:\n      - go run ./scripts/schemagen -kube-version {{.kubeconformValidationSchemasUpstreamKubeVersion}}\n",
		},
		{
			errContains: "found 2",
			name:        "fails on more than one pin",
			taskfile:    "vars:\n  kubeconformValidationSchemasUpstreamKubeVersion: \"1.36.3\"\n  kubeconformValidationSchemasUpstreamKubeVersion: \"1.35.0\"\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, pinned, err := replaceKubeVersionPin([]byte(tt.taskfile), "1.37.1")
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPinned, pinned)
			assert.Equal(t, tt.wantTask, string(got))
		})
	}
}
