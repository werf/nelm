package ts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	// minNodeVersion is the first Node.js with all the fixes to the permission model the render
	// sandbox relies on.
	minNodeVersion  = "26.12.1"
	nodeProbeMarker = "nelm-node-probe:"
)

// findNode returns the real path of the Node.js binary to render with: binaryPath if set, otherwise
// node from PATH. The candidate is run once with the environment of nelm, so that version manager
// shims, which need it, resolve to the real binary, which is then run with an empty environment.
func findNode(ctx context.Context, binaryPath string) (string, error) {
	candidate := binaryPath
	if candidate == "" {
		path, err := exec.LookPath("node")
		if err != nil {
			return "", newNodeNotFoundError(fmt.Sprintf("no node in PATH: %s", err), "")
		}

		candidate = path
	}

	var stdout, stderr bytes.Buffer

	// Preloaded modules, such as those NODE_OPTIONS can name, may print too.
	cmd := exec.CommandContext(ctx, candidate, "-p", "'"+nodeProbeMarker+"' + JSON.stringify({ execPath: process.execPath, version: process.versions.node })")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", newNodeNotFoundError(fmt.Sprintf("run %s: %s", candidate, err), stderr.String())
	}

	execPath, version, err := parseNodeProbeOutput(stdout.String())
	if err != nil {
		return "", newNodeNotFoundError(fmt.Sprintf("parse output of %s: %s", candidate, err), stderr.String())
	}

	parsedVersion, err := semver.StrictNewVersion(strings.TrimPrefix(version, "v"))
	if err != nil {
		return "", newNodeNotFoundError(fmt.Sprintf("parse version %q of %s: %s", version, execPath, err), "")
	}

	if parsedVersion.LessThan(semver.MustParse(minNodeVersion)) {
		return "", newNodeNotFoundError(fmt.Sprintf("found Node.js %s at %s", version, execPath), "")
	}

	return execPath, nil
}

func newNodeNotFoundError(found, stderr string) error {
	message := fmt.Sprintf("Node.js >= %s is required to render TypeScript charts, put it in PATH or pass --node-binary-path: %s", minNodeVersion, found)

	if stderr = strings.TrimSpace(stderr); stderr != "" {
		message += ": " + stderr
	}

	if strings.Contains(stderr, "libatomic.so.1") {
		message += "\nInstall the libatomic1 package (Debian/Ubuntu) or libatomic (RHEL/Fedora)."
	}

	return errors.New(message)
}

// parseNodeProbeOutput takes the path and the version from the last line of the output that starts
// with nodeProbeMarker.
func parseNodeProbeOutput(output string) (string, string, error) {
	var probeLine string

	for line := range strings.Lines(output) {
		if found, ok := strings.CutPrefix(strings.TrimSpace(line), nodeProbeMarker); ok {
			probeLine = found
		}
	}

	if probeLine == "" {
		return "", "", fmt.Errorf("no line starting with %q in the output: %q", nodeProbeMarker, output)
	}

	var probe struct {
		ExecPath string `json:"execPath"`
		Version  string `json:"version"`
	}

	if err := json.Unmarshal([]byte(probeLine), &probe); err != nil {
		return "", "", fmt.Errorf("decode %q: %w", probeLine, err)
	}

	if probe.ExecPath == "" || probe.Version == "" {
		return "", "", fmt.Errorf("no execPath or version in %q", probeLine)
	}

	return probe.ExecPath, probe.Version, nil
}
