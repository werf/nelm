package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/werf/nelm/v2/pkg/helm/pkg/repo/v1/repotest"
)

func TestPullCmd_LoopbackRegistryPlainHTTPByDefault(t *testing.T) {
	srv := repotest.NewTempServer(
		t,
		repotest.WithChartSourceGlob("testdata/testcharts/*.tgz*"),
	)
	defer srv.Stop()

	ociSrv, err := repotest.NewOCIServer(t, srv.Root())
	if err != nil {
		t.Fatal(err)
	}
	ociSrv.Run(t)

	outdir := t.TempDir()
	cmd := fmt.Sprintf("pull oci://%s/u/ocitestuser/oci-dependent-chart:0.1.0 -d '%s' --registry-config %s --content-cache %s",
		ociSrv.RegistryURL,
		outdir,
		filepath.Join(srv.Root(), "config.json"),
		t.TempDir(),
	)

	if _, _, err := executeActionCommand(cmd); err != nil {
		t.Fatalf("pull from a loopback registry without --plain-http failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outdir, "oci-dependent-chart-0.1.0.tgz")); err != nil {
		t.Errorf("expected chart file not found: %v", err)
	}
}
