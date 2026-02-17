package ts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

func BuildVendorBundle(ctx context.Context, chartPath string) error {
	denoBin, ok := os.LookupEnv("DENO_BIN")
	if !ok || denoBin == "" {
		denoBin = "deno"
	}

	cmd := exec.CommandContext(ctx, denoBin, "run", "-A", common.ChartTSBuildScript)
	cmd.Dir = chartPath + "/" + common.ChartTSSourceDir
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

func runApp(ctx context.Context, chartPath string, useVendorMap bool, entryPoint string, renderCtx map[string]any) (map[string]interface{}, error) {
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
	cmd.Dir = chartPath + "/" + common.ChartTSSourceDir
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

	jsonString, errJSON := waitForJSONString()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("wait process: %w", err)
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
