//go:build unix

package action

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChartCopy_ArchivePermissionsFollowUmask(t *testing.T) {
	host := newChartCopyTestRegistry(t)
	client := newChartCopyTestRegistryClient(t)

	chartData := buildChartCopyTestChart(t, map[string]string{"Chart.yaml": "apiVersion: v2\nname: app\nversion: 1.0.0\n"})
	_, err := client.Push(chartData, host+"/src/app:1.0.0")
	require.NoError(t, err)

	previousUmask := syscall.Umask(0o027)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	archivePath := filepath.Join(t.TempDir(), "chart.tar")
	require.NoError(t, ChartCopy(newChartPackTestContext(), "oci://"+host+"/src/app:1.0.0", archivePath, newChartCopyTestOptions(t)))

	info, err := os.Stat(archivePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}
