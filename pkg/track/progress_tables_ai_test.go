//go:build ai_tests

package track

import (
	"strings"
	"testing"

	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SetMaxTableWidth

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

func TestAI_SetMaxTableWidth_NoCap_BothGetSameWidth(t *testing.T) {
	b := &tablesBuilder{}
	b.SetMaxTableWidth(300)
	assert.Equal(t, 300, b.maxProgressTableWidth)
	assert.Equal(t, 300, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_LargeWidth_NoCap(t *testing.T) {
	b := &tablesBuilder{}
	b.SetMaxTableWidth(500)
	assert.Equal(t, 500, b.maxProgressTableWidth)
	assert.Equal(t, 500, b.maxLogEventTableWidth)
}

func TestAI_SetMaxTableWidth_Exactly200_NoCap(t *testing.T) {
	b := &tablesBuilder{}
	b.SetMaxTableWidth(200)
	assert.Equal(t, 200, b.maxProgressTableWidth)
	assert.Equal(t, 200, b.maxLogEventTableWidth)
}

// configuredMaxTableWidth

func TestAI_TablesBuilder_ConfiguredMaxTableWidthStored(t *testing.T) {
	b := &tablesBuilder{configuredMaxTableWidth: 180}
	assert.Equal(t, 180, b.configuredMaxTableWidth)
}

func TestAI_TablesBuilder_ZeroConfiguredMaxTableWidthMeansAuto(t *testing.T) {
	b := &tablesBuilder{configuredMaxTableWidth: 0}
	assert.Equal(t, 0, b.configuredMaxTableWidth)
}

// setProgressTableStyle column allocation

func TestAI_SetProgressTableStyle_InfoWiderThanResource(t *testing.T) {
	table := prtable.NewWriter()
	setProgressTableStyle(table, 192)

	// Render with content that exceeds any column width to force wrapping at WidthMax.
	longText := strings.Repeat("x", 300)
	table.AppendRow(prtable.Row{longText, "WAITING", longText})
	rendered := table.Render()

	// Measure actual column widths from the first data line.
	lines := strings.Split(rendered, "\n")
	require.NotEmpty(t, lines)

	resourceWidth, infoWidth := measureProgressColumnWidths(lines)
	assert.Greater(t, infoWidth, resourceWidth, "INFO column should be wider than RESOURCE column")
}

func TestAI_SetProgressTableStyle_InfoGetsMoreThanHalfWidth(t *testing.T) {
	tableWidth := 200
	table := prtable.NewWriter()
	setProgressTableStyle(table, tableWidth)

	longText := strings.Repeat("x", 300)
	table.AppendRow(prtable.Row{longText, "WAITING", longText})
	rendered := table.Render()

	lines := strings.Split(rendered, "\n")
	require.NotEmpty(t, lines)

	resourceWidth, infoWidth := measureProgressColumnWidths(lines)
	assert.Greater(t, infoWidth, tableWidth/2, "INFO should get more than half the total width")
	_ = resourceWidth
}

func TestAI_SetProgressTableStyle_NarrowTerminal_InfoStillPositive(t *testing.T) {
	table := prtable.NewWriter()
	setProgressTableStyle(table, 80)

	longText := strings.Repeat("x", 300)
	table.AppendRow(prtable.Row{longText, "WAITING", longText})
	rendered := table.Render()

	lines := strings.Split(rendered, "\n")
	require.NotEmpty(t, lines)

	_, infoWidth := measureProgressColumnWidths(lines)
	assert.Greater(t, infoWidth, 0, "INFO column must have positive width even on narrow terminal")
}

// measureProgressColumnWidths measures RESOURCE and INFO column widths from
// the rendered table by finding the first non-header content line.
func measureProgressColumnWidths(lines []string) (resourceWidth, infoWidth int) {
	for _, line := range lines {
		// Skip header line (all-caps words like RESOURCE STATE INFO).
		if strings.Contains(line, "RESOURCE") {
			continue
		}
		if len(line) == 0 {
			continue
		}
		// Trim trailing spaces; columns are separated by two spaces (PaddingRight="  ").
		parts := strings.SplitN(line, "  ", 3)
		if len(parts) < 3 {
			continue
		}
		resourceWidth = len(parts[0])
		// parts[1] is STATE, parts[2] is INFO (may itself contain "  " but we care about total).
		infoWidth = len(strings.TrimRight(parts[2], " "))
		return
	}
	return
}
