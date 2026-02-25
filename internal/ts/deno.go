package ts

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"

	"github.com/gookit/color"

	"github.com/werf/3p-helm/pkg/werf/ts"
	"github.com/werf/nelm/pkg/log"
)

const (
	renderInputFileName  = "input.yaml"
	renderOutputFileName = "output.yaml"
)

func runApp(ctx context.Context, bundleData []byte, renderDir string) error {
	args := []string{
		"run",
		"--no-remote",
		// deno permissions: allow read/write only for input and output files, deny all else.
		"--allow-read=" + renderInputFileName,
		"--allow-write=" + renderOutputFileName,
		"--deny-net",
		"--deny-env",
		"--deny-run",
		// write bundle data to Stdin
		"-",
		// pass input and output file names as arguments
		"--input-file=" + renderInputFileName,
		"--output-file=" + renderOutputFileName,
	}

	denoBin := tsbundle.GetDenoBinary()
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
