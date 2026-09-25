//go:build ai_tests

package spec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAI_normalizeNumbers_ErrorsOnUnrepresentableNumber(t *testing.T) {
	_, err := normalizeNumbers(json.Number("1e10000"))
	require.ErrorContains(t, err, "cannot be represented as int64 or float64")
}
