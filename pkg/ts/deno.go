package ts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofrs/flock"
	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/common"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/pkg/log"
)

var chartTSEntryPoints = [...]string{common.ChartTSEntryPointTS, common.ChartTSEntryPointJS}

func BundleChartsRecursive(ctx context.Context, chart *v2chart.Chart, path string, rebuildBundle bool, binaryPath string) error {
	if !hasTSFiles(chart) {
		return nil
	}

	denoBin, err := getDenoBinary(ctx, binaryPath)
	if err != nil {
		return fmt.Errorf("ensure Deno is available: %w", err)
	}

	return bundleChartsRecursive(ctx, chart, path, rebuildBundle, denoBin)
}

func bundleChartsRecursive(ctx context.Context, chart *v2chart.Chart, path string, rebuildBundle bool, denoBin string) error {
	entrypoint, bundle := getEntrypointAndBundle(chart.RuntimeFiles)

	if entrypoint != "" {
		if bundle == nil || rebuildBundle {
			log.Default.Info(ctx, "Bundle TypeScript for chart %q (entrypoint: %s)", chart.Name(), entrypoint)

			bundleRes, err := runDenoBundle(ctx, path, entrypoint, denoBin)
			if err != nil {
				return fmt.Errorf("build TypeScript bundle: %w", err)
			}

			if bundle != nil {
				chart.RemoveRuntimeFile(common.ChartTSBundleFile)
			}

			chart.AddRuntimeFile(common.ChartTSBundleFile, bundleRes)
		}
	}

	for _, dep := range chart.Dependencies() {
		depPath := filepath.Join(path, "charts", dep.Name())

		if _, err := os.Stat(depPath); err != nil {
			// Subchart loaded from .tgz or missing on disk — skip,
			// deno bundle needs a real directory to work with.
			continue
		}

		if err := bundleChartsRecursive(ctx, dep, depPath, rebuildBundle, denoBin); err != nil {
			return fmt.Errorf("process dependency %q: %w", dep.Name(), err)
		}
	}

	return nil
}

func getEntrypointAndBundle(files []*chartcommon.File) (string, *chartcommon.File) {
	entrypoint := findEntrypointInFiles(files)
	if entrypoint == "" {
		return "", nil
	}

	bundleFile, foundBundle := lo.Find(files, func(f *chartcommon.File) bool {
		return f.Name == common.ChartTSBundleFile
	})

	if !foundBundle {
		return entrypoint, nil
	}

	return entrypoint, bundleFile
}

func hasTSFiles(chart *v2chart.Chart) bool {
	entrypoint := findEntrypointInFiles(chart.RuntimeFiles)
	if entrypoint != "" {
		return true
	}

	for _, dep := range chart.Dependencies() {
		if hasTSFiles(dep) {
			return true
		}
	}

	return false
}

func findEntrypointInFiles(files []*chartcommon.File) string {
	sourceFiles := make(map[string][]byte)

	for _, f := range files {
		if strings.HasPrefix(f.Name, common.ChartTSSourceDir+"src/") {
			sourceFiles[strings.TrimPrefix(f.Name, common.ChartTSSourceDir)] = f.Data
		}
	}

	if len(sourceFiles) == 0 {
		return ""
	}

	for _, ep := range chartTSEntryPoints {
		if _, ok := sourceFiles[ep]; ok {
			return ep
		}
	}

	return ""
}

func getDenoBinary(ctx context.Context, binaryPath string) (string, error) {
	if binaryPath != "" {
		if _, err := os.Stat(binaryPath); err != nil {
			return "", fmt.Errorf("deno binary not found on path %q", binaryPath)
		}

		return binaryPath, nil
	}

	link, err := getDownloadLink()
	if err != nil {
		return "", fmt.Errorf("get download link: %w", err)
	}

	cacheDir, err := getDenoFolder(link)
	if err != nil {
		return "", fmt.Errorf("get Deno cache folder: %w", err)
	}

	binaryName := lo.Ternary(runtime.GOOS == "windows", "deno.exe", "deno")

	denoPath := filepath.Join(cacheDir, binaryName)
	if _, err := os.Stat(denoPath); err == nil {
		log.Default.Debug(ctx, "Using cached Deno binary: %s", denoPath)

		return denoPath, nil
	}

	lockFile := filepath.Join(cacheDir, "lock")

	fileLock := flock.New(lockFile)
	if err := fileLock.Lock(); err != nil {
		return "", fmt.Errorf("acquire lock on Deno cache: %w", err)
	}

	defer func() {
		if err := fileLock.Unlock(); err != nil {
			log.Default.Error(ctx, "release lock on Deno cache: %v", err)
		}

		if err := os.Remove(lockFile); err != nil {
			log.Default.Error(ctx, "remove Deno cache lock file: %v", err)
		}
	}()

	if _, err := os.Stat(denoPath); err == nil {
		log.Default.Debug(ctx, "Using cached Deno binary: %s", denoPath)

		return denoPath, nil
	}

	if err := downloadDeno(ctx, cacheDir, link); err != nil {
		return "", fmt.Errorf("download deno: %w", err)
	}

	return denoPath, nil
}

func runApp(ctx context.Context, bundleData []byte, renderDir, denoBin string) error {
	args := []string{
		"run",
		"--no-remote",
		// deno permissions: allow read/write only for input and output files, deny all else.
		"--allow-read=" + common.ChartTSInputFile,
		"--allow-write=" + common.ChartTSOutputFile,
		"--deny-net",
		"--deny-env",
		"--deny-run",
		// write bundle data to Stdin
		"-",
		// pass input and output file names as arguments
		"--input-file=" + common.ChartTSInputFile,
		"--output-file=" + common.ChartTSOutputFile,
	}

	cmd := exec.CommandContext(ctx, denoBin, args...)
	cmd.Dir = renderDir

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("get stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("get stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("get stderr pipe: %w", err)
	}

	stdinErrChan := make(chan error, 1)
	go func() {
		defer func() {
			_ = stdinPipe.Close()
		}()

		_, writeErr := stdinPipe.Write(bundleData)
		stdinErrChan <- writeErr
	}()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	stdoutErrChan := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			text := scanner.Text()
			log.Default.Debug(ctx, text)
		}

		stdoutErrChan <- scanner.Err()
	}()

	stderrErrChan := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log.Default.Error(ctx, "error from deno app: %s", scanner.Text())
		}

		stderrErrChan <- scanner.Err()
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("wait process: %w", err)
	}

	if err := <-stdinErrChan; err != nil {
		return fmt.Errorf("write bundle data to stdinPipe: %w", err)
	}

	if err := <-stdoutErrChan; err != nil {
		return fmt.Errorf("read stdout: %w", err)
	}

	if err := <-stderrErrChan; err != nil {
		return fmt.Errorf("read stderr: %w", err)
	}

	return nil
}

func runDenoBundle(ctx context.Context, chartPath, entryPoint, denoBin string) ([]uint8, error) {
	cmd := exec.CommandContext(ctx, denoBin, "bundle", entryPoint)
	cmd.Dir = filepath.Join(chartPath, common.ChartTSSourceDir)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			log.Default.Error(ctx, "run deno bundle error: %s", exitErr)
		}

		return nil, fmt.Errorf("get deno build output: %w", err)
	}

	return output, nil
}
