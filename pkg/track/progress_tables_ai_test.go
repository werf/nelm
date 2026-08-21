//go:build ai_tests

package track

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAI_SetMaxTableWidth_DefaultsTo140WhenZero(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(0)

	assert.Equal(t, 140, b.maxProgressTableWidth)
	assert.Equal(t, 140, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_DefaultsTo140WhenNegative(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(-5)

	assert.Equal(t, 140, b.maxProgressTableWidth)
	assert.Equal(t, 140, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_UsesProvidedWidthWhenPositive(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(120)

	assert.Equal(t, 120, b.maxProgressTableWidth)
	assert.Equal(t, 120, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_CapsProgressTableAt200(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(300)

	assert.Equal(t, 200, b.maxProgressTableWidth)
}

func TestAI_SetMaxTableWidth_CapsLogEventTableAt250(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(300)

	assert.Equal(t, 250, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_ProgressCapIsLowerThanLogEventCap(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(300)

	assert.Less(t, b.maxProgressTableWidth, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_ExactAtProgressCap(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(200)

	assert.Equal(t, 200, b.maxProgressTableWidth)
	assert.Equal(t, 200, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_ExactAtLogEventCap(t *testing.T) {
	b := &tablesBuilder{}

	b.SetMaxTableWidth(250)

	assert.Equal(t, 200, b.maxProgressTableWidth)
	assert.Equal(t, 250, b.maxLogEventTableWidth)
}

func TestAI_NewTablesBuilder_ConfiguredMaxTableWidthStored(t *testing.T) {
	b := &tablesBuilder{
		configuredMaxTableWidth: 180,
	}

	assert.Equal(t, 180, b.configuredMaxTableWidth)
}

func TestAI_NewTablesBuilder_ZeroConfiguredMaxTableWidthMeansAuto(t *testing.T) {
	b := &tablesBuilder{
		configuredMaxTableWidth: 0,
	}

	assert.Equal(t, 0, b.configuredMaxTableWidth)
}
