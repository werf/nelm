//go:build ai_tests

package featgate

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAI_CaseInsensitiveConditionTracking_EnvVarName(t *testing.T) {
	assert.Equal(t, "NELM_FEAT_CASE_INSENSITIVE_CONDITION_TRACKING", FeatGateCaseInsensitiveConditionTracking.EnvVarName())
	assert.Equal(t, "case-insensitive-condition-tracking", FeatGateCaseInsensitiveConditionTracking.Name)
}

func TestAI_CaseInsensitiveConditionTracking_OnlyTrueEnables(t *testing.T) {
	assert.False(t, FeatGateCaseInsensitiveConditionTracking.Default())

	for _, value := range []string{"", "1", "yes", "TRUE", "True", "false"} {
		t.Setenv(FeatGateCaseInsensitiveConditionTracking.EnvVarName(), value)
		assert.False(t, FeatGateCaseInsensitiveConditionTracking.Enabled(), "value %q must not enable the gate", value)
	}

	t.Setenv(FeatGateCaseInsensitiveConditionTracking.EnvVarName(), "true")
	assert.True(t, FeatGateCaseInsensitiveConditionTracking.Enabled())
}

func TestAI_CaseInsensitiveConditionTracking_Registered(t *testing.T) {
	_, found := lo.Find(FeatGates, func(fg *FeatGate) bool {
		return fg == FeatGateCaseInsensitiveConditionTracking
	})

	require.True(t, found, "gate must be registered in FeatGates so CLI usage and reference docs pick it up")
}
