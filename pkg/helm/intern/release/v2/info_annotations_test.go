package v2

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfoAnnotationsMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		info     Info
		expected string
	}{
		{
			name:     "populated",
			info:     Info{Annotations: map[string]string{"packages.deckhouse.io/managed-by": "deckhouse", "checksum": "123"}},
			expected: `{"annotations":{"packages.deckhouse.io/managed-by":"deckhouse","checksum":"123"}}`,
		},
		{
			name:     "nil omitted",
			info:     Info{},
			expected: `{}`,
		},
		{
			name:     "empty omitted",
			info:     Info{Annotations: map[string]string{}},
			expected: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(&tt.info)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestInfoAnnotationsUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "populated",
			input:    `{"annotations":{"packages.deckhouse.io/managed-by":"deckhouse","checksum":"123"}}`,
			expected: map[string]string{"packages.deckhouse.io/managed-by": "deckhouse", "checksum": "123"},
		},
		{
			name:     "with empty time fields",
			input:    `{"first_deployed":"","last_deployed":"","annotations":{"key":"value"}}`,
			expected: map[string]string{"key": "value"},
		},
		{
			name:  "null",
			input: `{"annotations":null}`,
		},
		{
			name:     "empty",
			input:    `{"annotations":{}}`,
			expected: map[string]string{},
		},
		{
			name:    "invalid value",
			input:   `{"annotations":{"key":1}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info Info
			err := json.Unmarshal([]byte(tt.input), &info)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, info.Annotations)
		})
	}
}

func TestInfoAnnotationsRoundTrip(t *testing.T) {
	original := Info{
		FirstDeployed: time.Date(2025, 10, 8, 12, 0, 0, 0, time.UTC),
		Description:   "Test release",
		Annotations:   map[string]string{"packages.deckhouse.io/managed-by": "deckhouse"},
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Info
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Annotations, decoded.Annotations)
	assert.Equal(t, original.FirstDeployed.Unix(), decoded.FirstDeployed.Unix())
	assert.Equal(t, original.Description, decoded.Description)
}
