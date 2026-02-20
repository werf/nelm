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

	"github.com/werf/3p-helm/pkg/werf/ts"
	"github.com/werf/nelm/pkg/log"
)

const (
	// renderResultPrefix is the prefix for the rendered output.
	renderResultPrefix = "NELM_RENDER_RESULT:"
)

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

	denoBin := tsbundle.GetDenoBinary()
	cmd := exec.CommandContext(ctx, denoBin, args...)
	cmd.Dir = filepath.Join(chartPath, tsbundle.ChartTSSourceDir)
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

			if strings.HasPrefix(text, renderResultPrefix) {
				_, str, found := strings.Cut(text, renderResultPrefix)
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
