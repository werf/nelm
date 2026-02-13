package ts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

func renderDenoFiles(ctx context.Context, chart *helmchart.Chart, renderedValues chartutil.Values, chartDir string, rebuildVendor bool) (map[string]string, error) {
	mergedFiles := slices.Concat(chart.RuntimeFiles, chart.RuntimeDepsFiles)
	tsRootDir := chartDir + "/" + common.ChartTSSourceDir

	var (
		hasNodeModules bool
		useVendorMap   bool
		vendorFiles    []*helmchart.File
	)
	for _, file := range mergedFiles {
		if strings.HasPrefix(file.Name, common.ChartTSSourceDir+"node_modules/") {
			hasNodeModules = true
		} else if strings.HasPrefix(file.Name, common.ChartTSVendorBundleDir) {
			vendorFiles = append(vendorFiles, file)
		} else if file.Name == common.ChartTSSourceDir+common.ChartTSVendorMap {
			useVendorMap = true
		}
	}

	if hasNodeModules && (rebuildVendor || len(vendorFiles) == 0) {
		err := buildDenoVendorBundle(ctx, tsRootDir)
		if err != nil {
			return nil, fmt.Errorf("build deno vendor bundle: %w", err)
		}
	}

	sourceFiles := extractSourceFiles(mergedFiles)
	if len(sourceFiles) == 0 {
		return map[string]string{}, nil
	}

	entrypoint := findEntrypointInFiles(sourceFiles)
	if entrypoint == "" {
		return map[string]string{}, nil
	}

	result, err := runDenoApp(ctx, tsRootDir, useVendorMap, entrypoint, buildRenderContext(renderedValues, chart))
	if err != nil {
		return nil, fmt.Errorf("run deno app: %w", err)
	}

	if result == nil {
		return map[string]string{}, nil
	}

	yamlOutput, err := convertRenderResultToYAML(result)
	if err != nil {
		return nil, fmt.Errorf("convert render result to yaml: %w", err)
	}

	if strings.TrimSpace(yamlOutput) == "" {
		return map[string]string{}, nil
	}

	return map[string]string{
		path.Join(common.ChartTSSourceDir, entrypoint): yamlOutput,
	}, nil
}

func buildDenoVendorBundle(ctx context.Context, tsRootDir string) error {
	denoBin, ok := os.LookupEnv("DENO_BIN")
	if !ok || denoBin == "" {
		denoBin = "deno"
	}

	cmd := exec.CommandContext(ctx, denoBin, "run", "-A", "build.ts")
	cmd.Dir = tsRootDir
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			_, _ = os.Stderr.Write(exitErr.Stderr)
		}

		return fmt.Errorf("get deno build output: %w", err)
	}

	return nil
}

func runDenoApp(ctx context.Context, tsRootDir string, useVendorMap bool, entryPoint string, renderCtx map[string]any) (map[string]interface{}, error) {
	denoBin, ok := os.LookupEnv("DENO_BIN")
	if !ok || denoBin == "" {
		denoBin = "deno"
	}

	args := []string{"run"}
	if useVendorMap {
		args = append(args, "--import-map", common.ChartTSVendorMap)
	}

	args = append(args, entryPoint)

	cmd := exec.CommandContext(ctx, denoBin, args...)
	cmd.Dir = tsRootDir
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("get stdin pipe: %w", err)
	}

	go func() {
		defer func() {
			_ = stdin.Close()
		}()

		_ = json.NewEncoder(stdin).Encode(renderCtx)
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

		return "", errors.New("render output not found")
	}

	jsonString, errJson := waitForJSONString()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("wait process: %w", err)
	}

	if errJson != nil {
		return nil, fmt.Errorf("wait for render output: %w", errJson)
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
