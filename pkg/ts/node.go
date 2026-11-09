package ts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const minNodeMajorVersion = 26

// findNode returns the real path of the Node.js binary to render with: binaryPath if set, otherwise
// node from PATH. The candidate is run once with the environment of nelm, so that version manager
// shims, which need it, resolve to the real binary, which is then run with an empty environment.
func findNode(ctx context.Context, binaryPath string) (string, error) {
	candidate := binaryPath
	if candidate == "" {
		path, err := exec.LookPath("node")
		if err != nil {
			return "", newNodeNotFoundError("no node in PATH", "")
		}

		candidate = path
	}

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, candidate, "-p", "process.execPath + ' ' + process.versions.node")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", newNodeNotFoundError(fmt.Sprintf("run %s: %s", candidate, err), stderr.String())
	}

	execPath, version, err := parseNodeExecPathAndVersion(stdout.String())
	if err != nil {
		return "", newNodeNotFoundError(fmt.Sprintf("parse output of %s: %s", candidate, err), stderr.String())
	}

	major, err := nodeMajorVersion(version)
	if err != nil {
		return "", newNodeNotFoundError(fmt.Sprintf("parse version of %s: %s", execPath, err), "")
	}

	if major < minNodeMajorVersion {
		return "", newNodeNotFoundError(fmt.Sprintf("found Node.js %s at %s", version, execPath), "")
	}

	return execPath, nil
}

func newNodeNotFoundError(found, stderr string) error {
	message := fmt.Sprintf("Node.js >= %d is required to render TypeScript charts, put it in PATH or pass --node-binary-path: %s", minNodeMajorVersion, found)

	if stderr = strings.TrimSpace(stderr); stderr != "" {
		message += ": " + stderr
	}

	if strings.Contains(stderr, "libatomic.so.1") {
		message += "\nInstall the libatomic1 package (Debian/Ubuntu) or libatomic (RHEL/Fedora)."
	}

	return errors.New(message)
}

func nodeMajorVersion(version string) (int, error) {
	major, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), ".")

	number, err := strconv.Atoi(major)
	if err != nil {
		return 0, fmt.Errorf("parse major version of %q: %w", version, err)
	}

	return number, nil
}

// parseNodeExecPathAndVersion parses "<execPath> <version>": the path may contain spaces, the version
// does not.
func parseNodeExecPathAndVersion(output string) (string, string, error) {
	output = strings.TrimSpace(output)

	separator := strings.LastIndex(output, " ")
	if separator <= 0 || separator == len(output)-1 {
		return "", "", fmt.Errorf("want \"<path> <version>\", got %q", output)
	}

	return output[:separator], output[separator+1:], nil
}
