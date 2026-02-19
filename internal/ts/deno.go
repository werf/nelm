package ts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/gookit/color"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

func getDenoBinary() string {
	if denoBin, ok := os.LookupEnv("DENO_BIN"); ok && denoBin != "" {
		return denoBin
	}

	return "deno"
}

func BuildBundleToFile(ctx context.Context, chartPath string) error {
	srcDir := filepath.Join(chartPath, common.ChartTSSourceDir, "src")

	files, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read source directory %q: %w", srcDir, err)
	}

	entrypoint := findEntrypointInDir(files)
	if entrypoint == "" {
		return fmt.Errorf("entry point not found in source directory")
	}

	bundle, err := buildBundle(ctx, chartPath, entrypoint)
	if err != nil {
		return fmt.Errorf("build bundle: %w", err)
	}

	if err := saveBundleToFile(chartPath, bundle); err != nil {
		return fmt.Errorf("save bundle: %w", err)
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Bundled: ")+"%s - %s", "dist/bundle.js", humanize.Bytes(uint64(len(bundle))))

	return nil
}

func buildBundle(ctx context.Context, chartPath, entryPoint string) ([]uint8, error) {
	denoBin := getDenoBinary()
	cmd := exec.CommandContext(ctx, denoBin, "bundle", entryPoint)
	cmd.Dir = filepath.Join(chartPath, common.ChartTSSourceDir)

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

func saveBundleToFile(chartPath string, bundle []byte) error {
	distDir := filepath.Join(chartPath, common.ChartTSSourceDir, "dist")
	if err := os.MkdirAll(distDir, 0o775); err != nil {
		return fmt.Errorf("mkdir %q: %w", distDir, err)
	}

	bundlePath := filepath.Join(chartPath, common.ChartTSVendorBundleFile)
	if err := os.WriteFile(bundlePath, bundle, 0o644); err != nil {
		return fmt.Errorf("write vendor bundle to file %q: %w", bundlePath, err)
	}

	return nil
}

func runApp(ctx context.Context, chartPath string, bundleData []byte, renderCtx string) (map[string]interface{}, error) {
	args := []string{
		"run",
		"--no-remote",
		"--deny-read",
		"--deny-write",
		"--deny-net",
		"--deny-env",
		"--deny-run",
		"-",
		"--ctx",
		renderCtx,
	}

	denoBin := getDenoBinary()
	cmd := exec.CommandContext(ctx, denoBin, args...)
	cmd.Dir = filepath.Join(chartPath, common.ChartTSSourceDir)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("get stdin pipe: %w", err)
	}

	stdinErrChan := make(chan error, 1)
	go func() {
		defer func() {
			_ = stdin.Close()
		}()

		_, writeErr := stdin.Write(bundleData)
		stdinErrChan <- writeErr
	}()

	reader, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	waitForJSONString := func() (string, error) {
		scanner := bufio.NewScanner(reader)
		// Increase buffer size to handle large JSON outputs (up to 10MB)
		const maxScannerBuffer = 10 * 1024 * 1024
		scanner.Buffer(make([]byte, 64*1024), maxScannerBuffer)

		for scanner.Scan() {
			text := scanner.Text()
			log.Default.Debug(ctx, text)

			if strings.HasPrefix(text, common.ChartTSRenderResultPrefix) {
				_, str, found := strings.Cut(text, common.ChartTSRenderResultPrefix)
				if found {
					return str, nil
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("scan stdout: %w", err)
		}

		return "", errors.New("render output not found")
	}

	jsonString, errJSON := waitForJSONString()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("wait process: %w", err)
	}

	if stdinErr := <-stdinErrChan; stdinErr != nil {
		return nil, fmt.Errorf("write bundle data to stdin: %w", stdinErr)
	}

	if errJSON != nil {
		return nil, fmt.Errorf("wait for render output: %w", errJSON)
	}

	if jsonString == "" {
		return nil, fmt.Errorf("unexpected render output format")
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonString), &result); err != nil {
		return nil, fmt.Errorf("unmarshal render output: %w", err)
	}

	return result, nil
}
