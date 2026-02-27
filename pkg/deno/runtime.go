package deno

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
	"github.com/gookit/color"
	"github.com/samber/lo"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/werf/ts"
	"github.com/werf/nelm/pkg/log"
)

const (
	// ChartTSBundleFile is the path to the bundle in a Helm chart.
	ChartTSBundleFile = ChartTSSourceDir + "dist/bundle.js"
	// ChartTSEntryPointJS is the JavaScript entry point path.
	ChartTSEntryPointJS = "src/index.js"
	// ChartTSEntryPointTS is the TypeScript entry point path.
	ChartTSEntryPointTS = "src/index.ts"
	// ChartTSSourceDir is the directory containing TypeScript sources in a Helm chart.
	ChartTSSourceDir = "ts/"
	// RenderInputFileName is the name of the input file with context for the Deno app.
	RenderInputFileName = "input.yaml"
	// RenderOutputFileName is the name of the output file with rendered manifests from the Deno app.
	RenderOutputFileName = "output.yaml"
)

var (
	_ ts.Bundler = (*DenoRuntime)(nil)

	ChartTSEntryPoints = [...]string{ChartTSEntryPointTS, ChartTSEntryPointJS}
)

type DenoRuntime struct {
	binPath string
	rebuild bool
}

func NewDenoRuntime(rebuild bool, opts DenoRuntimeOptions) *DenoRuntime {
	return &DenoRuntime{binPath: opts.BinaryPath, rebuild: rebuild}
}

func (rt *DenoRuntime) BundleChartsRecursive(ctx context.Context, chart *helmchart.Chart, path string) error {
	entrypoint, bundle := GetEntrypointAndBundle(chart.RuntimeFiles)
	if entrypoint == "" {
		return nil
	}

	if bundle == nil || rt.rebuild {
		if err := rt.ensureBinary(ctx); err != nil {
			return fmt.Errorf("ensure Deno is available: %w", err)
		}

		log.Default.Info(ctx, "Bundle TypeScript for chart %q (entrypoint: %s)", chart.Name(), entrypoint)

		bundleRes, err := rt.runDenoBundle(ctx, path, entrypoint)
		if err != nil {
			return fmt.Errorf("build TypeScript bundle: %w", err)
		}

		if rt.rebuild && bundle != nil {
			chart.RemoveRuntimeFile(ChartTSBundleFile)
		}

		chart.AddRuntimeFile(ChartTSBundleFile, bundleRes)
	}

	deps := chart.Dependencies()
	if len(deps) == 0 {
		return nil
	}

	for _, dep := range deps {
		depPath := filepath.Join(path, "charts", dep.Name())

		if _, err := os.Stat(depPath); err != nil {
			// Subchart loaded from .tgz or missing on disk — skip,
			// deno bundle needs a real directory to work with.
			continue
		}

		if err := rt.BundleChartsRecursive(ctx, dep, depPath); err != nil {
			return fmt.Errorf("process dependency %q: %w", dep.Name(), err)
		}
	}

	return nil
}

func (rt *DenoRuntime) RunApp(ctx context.Context, bundleData []byte, renderDir string) error {
	if err := rt.ensureBinary(ctx); err != nil {
		return fmt.Errorf("ensure Deno is available: %w", err)
	}

	args := []string{
		"run",
		"--no-remote",
		// deno permissions: allow read/write only for input and output files, deny all else.
		"--allow-read=" + RenderInputFileName,
		"--allow-write=" + RenderOutputFileName,
		"--deny-net",
		"--deny-env",
		"--deny-run",
		// write bundle data to Stdin
		"-",
		// pass input and output file names as arguments
		"--input-file=" + RenderInputFileName,
		"--output-file=" + RenderOutputFileName,
	}

	denoBin := rt.getBinaryPath()
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
			log.Default.Debug(ctx, color.Style{color.Red}.Sprint(scanner.Text()))
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

func (rt *DenoRuntime) ensureBinary(ctx context.Context) error {
	if denoBin := rt.getBinaryPath(); denoBin != "" {
		if _, err := os.Stat(denoBin); err != nil {
			return fmt.Errorf("deno binary not found on path %q", denoBin)
		}

		return nil
	}

	link, err := getDownloadLink()
	if err != nil {
		return fmt.Errorf("get download link: %w", err)
	}

	cacheDir, err := getDenoFolder(link)
	if err != nil {
		return fmt.Errorf("get Deno cache folder: %w", err)
	}

	binaryName := lo.Ternary(runtime.GOOS == "windows", "deno.exe", "deno")

	denoPath := filepath.Join(cacheDir, binaryName)
	if _, err := os.Stat(denoPath); err == nil {
		rt.setBinaryPath(denoPath)
		log.Default.Debug(ctx, "Using cached Deno binary: %s", denoPath)

		return nil
	}

	lockFile := filepath.Join(cacheDir, "lock")

	fileLock := flock.New(lockFile)
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("acquire lock on Deno cache: %w", err)
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
		rt.setBinaryPath(denoPath)
		log.Default.Debug(ctx, "Using cached Deno binary: %s", denoPath)

		return nil
	}

	if err := downloadDeno(ctx, cacheDir, link); err != nil {
		return fmt.Errorf("download deno: %w", err)
	}

	rt.setBinaryPath(denoPath)

	return nil
}

func (rt *DenoRuntime) getBinaryPath() string {
	return rt.binPath
}

func (rt *DenoRuntime) runDenoBundle(ctx context.Context, chartPath, entryPoint string) ([]uint8, error) {
	denoBin := rt.getBinaryPath()
	cmd := exec.CommandContext(ctx, denoBin, "bundle", entryPoint)
	cmd.Dir = filepath.Join(chartPath, ChartTSSourceDir)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			_, _ = os.Stderr.Write(exitErr.Stderr)
		}

		return nil, fmt.Errorf("get deno build output: %w", err)
	}

	return output, nil
}

func (rt *DenoRuntime) setBinaryPath(path string) {
	rt.binPath = path
}

type DenoRuntimeOptions struct {
	BinaryPath string
}

func GetEntrypointAndBundle(files []*helmchart.File) (string, *helmchart.File) {
	entrypoint := findEntrypointInFiles(files)
	if entrypoint == "" {
		return "", nil
	}

	bundleFile, foundBundle := lo.Find(files, func(f *helmchart.File) bool {
		return f.Name == ChartTSBundleFile
	})

	if !foundBundle {
		return entrypoint, nil
	}

	return entrypoint, bundleFile
}

func findEntrypointInFiles(files []*helmchart.File) string {
	sourceFiles := make(map[string][]byte)

	for _, f := range files {
		if strings.HasPrefix(f.Name, ChartTSSourceDir+"src/") {
			sourceFiles[strings.TrimPrefix(f.Name, ChartTSSourceDir)] = f.Data
		}
	}

	if len(sourceFiles) == 0 {
		return ""
	}

	for _, ep := range ChartTSEntryPoints {
		if _, ok := sourceFiles[ep]; ok {
			return ep
		}
	}

	return ""
}
