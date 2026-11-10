package ts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/samber/lo"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/log"
)

const (
	maxChartOutputBytes = 10 << 20
	// maxChartOutputLineBytes keeps a long line, such as a large JSON written at once, from slowing
	// the logger down, as bufio.Scanner did before.
	maxChartOutputLineBytes = 64 << 10
	// maxStderrExcerptBytes limits the stderr of a failed chart put into the error, which is the only
	// place it is seen with the default log level of chart render.
	maxStderrExcerptBytes = 64 << 10
)

// chartOutputWriter passes what the chart writes to onLine line by line, cutting lines longer than
// maxChartOutputLineBytes, and stops the chart once it has written more than maxChartOutputBytes.
type chartOutputWriter struct {
	exceeded       bool
	name           string
	onLine         func(line string)
	partial        []byte
	stop           context.CancelFunc
	truncatedBytes int
	written        int
}

func (w *chartOutputWriter) Write(p []byte) (int, error) {
	if w.exceeded {
		return len(p), nil
	}

	written := len(p)

	if allowed := maxChartOutputBytes - w.written; len(p) > allowed {
		w.exceeded = true
		w.stop()

		p = p[:allowed]
	}

	w.written += len(p)

	for len(p) > 0 {
		lineEnd := bytes.IndexByte(p, '\n')

		chunk := p
		if lineEnd >= 0 {
			chunk = p[:lineEnd]
		}

		kept := min(len(chunk), max(maxChartOutputLineBytes-len(w.partial), 0))
		w.partial = append(w.partial, chunk[:kept]...)
		w.truncatedBytes += len(chunk) - kept

		if lineEnd < 0 {
			break
		}

		w.flush()

		p = p[lineEnd+1:]
	}

	if w.exceeded {
		w.partial = nil
		w.truncatedBytes = 0
	}

	return written, nil
}

func (w *chartOutputWriter) flush() {
	if len(w.partial) == 0 && w.truncatedBytes == 0 {
		return
	}

	line := string(w.partial)
	if w.truncatedBytes > 0 {
		line += fmt.Sprintf(" [line truncated, %d more bytes]", w.truncatedBytes)
	}

	w.onLine(line)

	w.partial = nil
	w.truncatedBytes = 0
}

// stderrExcerpt keeps the beginning and the end of what it is given, up to maxStderrExcerptBytes.
type stderrExcerpt struct {
	head  []byte
	tail  []byte
	total int
}

func (e *stderrExcerpt) String() string {
	if e.total <= maxStderrExcerptBytes {
		return strings.TrimRight(string(e.head)+string(e.tail), "\n")
	}

	tail := e.tail[len(e.tail)-maxStderrExcerptBytes/2:]
	skipped := e.total - len(e.head) - len(tail)

	return strings.TrimRight(fmt.Sprintf("%s\n[... %d bytes of stderr skipped ...]\n%s", e.head, skipped, tail), "\n")
}

func (e *stderrExcerpt) add(text string) {
	const half = maxStderrExcerptBytes / 2

	e.total += len(text)

	headRoom := min(half-len(e.head), len(text))
	e.head = append(e.head, text[:headRoom]...)
	e.tail = append(e.tail, text[headRoom:]...)

	if len(e.tail) > 2*half {
		e.tail = slices.Clone(e.tail[len(e.tail)-half:])
	}
}

func RenderChart(ctx context.Context, chart helmchart.Accessor, renderedValues chartcommon.Values, rebuildBundle bool, chartPath, tempDirPath, nodeBinaryPath string) (map[string]string, error) {
	if !hasTSFiles(chart) {
		return map[string]string{}, nil
	}

	if err := BundleChartsRecursive(ctx, chart, chartPath, rebuildBundle); err != nil {
		return nil, fmt.Errorf("bundle chart recursive: %w", err)
	}

	nodeBin, err := findNode(ctx, nodeBinaryPath)
	if err != nil {
		return nil, fmt.Errorf("find Node.js: %w", err)
	}

	allRendered, err := renderChartRecursive(ctx, chart, renderedValues, chart.Name(), chartPath, tempDirPath, nodeBin)
	if err != nil {
		return nil, fmt.Errorf("render chart recursive: %w", err)
	}

	return allRendered, nil
}

func renderChartRecursive(ctx context.Context, chart helmchart.Accessor, values chartcommon.Values, pathPrefix, chartPath, tempDirPath, nodeBin string) (map[string]string, error) {
	log.Default.Debug(ctx, "Rendering TypeScript for chart %q (path prefix: %s)", chart.Name(), pathPrefix)

	results := make(map[string]string)
	entrypoint, bundle := getEntrypointAndBundle(chart.RuntimeFiles())

	if bundle != nil {
		content, err := renderChart(ctx, bundle, chart, values, tempDirPath, nodeBin)
		if err != nil {
			return nil, fmt.Errorf("render files for chart %q: %w", chart.Name(), err)
		}

		if content != "" {
			outputPath := path.Join(pathPrefix, common.ChartTSSourceDir, lo.CoalesceOrEmpty(entrypoint, bundle.Name))
			results[outputPath] = content
			log.Default.Debug(ctx, "Rendered TypeScript output for chart %s: %s", chart.Name(), outputPath)
		}
	}

	for _, dep := range chart.Dependencies() {
		depAcc, err := helmchart.NewAccessor(dep)
		if err != nil {
			return nil, fmt.Errorf("create accessor for dependency: %w", err)
		}

		log.Default.Debug(ctx, "Processing dependency %q for chart %q", depAcc.Name(), chart.Name())

		depResults, err := renderChartRecursive(
			ctx,
			depAcc,
			scopeValuesForSubchart(values, depAcc.Name(), depAcc),
			path.Join(pathPrefix, "charts", depAcc.Name()),
			filepath.Join(chartPath, "charts", depAcc.Name()),
			tempDirPath,
			nodeBin,
		)
		if err != nil {
			return nil, fmt.Errorf("render dependency %q: %w", depAcc.Name(), err)
		}

		results = lo.Assign(results, depResults)
	}

	return results, nil
}

func renderChart(ctx context.Context, bundle *chartcommon.File, chart helmchart.Accessor, renderedValues chartcommon.Values, tempDirPath, nodeBin string) (string, error) {
	renderDir := filepath.Join(tempDirPath, "typescript-render", chart.ChartFullPath())
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		return "", fmt.Errorf("create temp dir for render context: %w", err)
	}

	renderFiles := []string{common.ChartTSInputFile, common.ChartTSOutputFile}

	// The temp dir may be reused: whatever is left there, symlinks included, must not be written
	// through or passed to Node.js in place of the files created here.
	for _, file := range renderFiles {
		if err := os.Remove(filepath.Join(renderDir, file)); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove leftover %s: %w", file, err)
		}
	}

	// The input holds the values, decrypted secrets included.
	defer func() {
		for _, file := range renderFiles {
			if err := os.Remove(filepath.Join(renderDir, file)); err != nil && !os.IsNotExist(err) {
				log.Default.Warn(ctx, "Unable to remove %s: %s", filepath.Join(renderDir, file), err)
			}
		}
	}()

	if err := writeInputRenderContext(renderedValues, chart, renderDir); err != nil {
		return "", fmt.Errorf("write input render context: %w", err)
	}

	if err := runBundle(ctx, nodeBin, renderDir, bundle.Data); err != nil {
		return "", fmt.Errorf("run bundle: %w", err)
	}

	resultBytes, err := os.ReadFile(filepath.Join(renderDir, common.ChartTSOutputFile))
	if err != nil {
		return "", fmt.Errorf("read output file: %w", err)
	}

	return strings.TrimSpace(string(resultBytes)), nil
}

func scopeValuesForSubchart(parentValues chartcommon.Values, subchartName string, subchart helmchart.Accessor) chartcommon.Values {
	scoped := chartcommon.Values{
		"Chart":  buildChartMetadata(subchart),
		"Values": map[string]any{},
	}

	for _, key := range []string{"Capabilities", "Release", "Runtime"} {
		if v, ok := parentValues[key]; ok {
			scoped[key] = v
		}
	}

	if parentVals := parentValues["Values"]; parentVals != nil {
		var valuesMap map[string]any
		switch v := parentVals.(type) {
		case map[string]any:
			valuesMap = v
		case chartcommon.Values:
			valuesMap = v
		}

		if valuesMap != nil {
			if subVals, ok := valuesMap[subchartName]; ok {
				scoped["Values"] = subVals
			}
		}
	}

	files := make(map[string]any, len(subchart.Files()))
	for _, f := range subchart.Files() {
		files[f.Name] = f.Data
	}

	scoped["Files"] = files

	return scoped
}

func writeInputRenderContext(renderedValues chartcommon.Values, chart helmchart.Accessor, renderDir string) error {
	renderContext := renderedValues.AsMap()

	if valuesInterface, ok := renderContext["Values"]; ok {
		if chartValues, ok := valuesInterface.(chartcommon.Values); ok {
			renderContext["Values"] = chartValues.AsMap()
		}
	}

	renderContext["Chart"] = buildChartMetadata(chart)

	files := make(map[string]any, len(chart.Files()))
	for _, file := range chart.Files() {
		files[file.Name] = file.Data
	}

	renderContext["Files"] = files

	yamlInput, err := yaml.Marshal(renderContext)
	if err != nil {
		return fmt.Errorf("marshal render context to yaml: %w", err)
	}

	if err := os.WriteFile(filepath.Join(renderDir, common.ChartTSInputFile), yamlInput, 0o644); err != nil {
		return fmt.Errorf("write render context to file: %w", err)
	}

	return nil
}

func buildChartMetadata(chart helmchart.Accessor) map[string]any {
	metadata := chart.MetadataAsMap()
	if metadata == nil {
		return map[string]any{
			"Name":    chart.Name(),
			"Version": "",
		}
	}

	return metadata
}

// runBundle runs the bundle, passed through stdin, on the input file in renderDir. Node.js may read
// only the input file and write only the output file, and gets no environment. This keeps a chart
// and its dependencies from reaching the rest of the machine by accident, and is not meant to
// contain malicious code: the Node.js permission model does not cover everything, such as sending
// signals to other processes.
func runBundle(ctx context.Context, nodeBin, renderDir string, bundle []byte) error {
	absRenderDir, err := filepath.Abs(renderDir)
	if err != nil {
		return fmt.Errorf("get absolute path of %s: %w", renderDir, err)
	}

	resolvedRenderDir, err := filepath.EvalSymlinks(absRenderDir)
	if err != nil {
		return fmt.Errorf("resolve symlinks in %s: %w", absRenderDir, err)
	}

	inputPath := filepath.Join(resolvedRenderDir, common.ChartTSInputFile)
	outputPath := filepath.Join(resolvedRenderDir, common.ChartTSOutputFile)

	for _, path := range []string{inputPath, outputPath} {
		if strings.Contains(path, "*") {
			return fmt.Errorf("path %q contains \"*\", which Node.js would take as a wildcard in its permissions: use a --temp-dir and chart names without \"*\"", path)
		}
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	cmd := exec.CommandContext(runCtx, nodeBin, //nolint:gosec // G204: nodeBin is the Node.js found in PATH or the one the user passed explicitly
		"--permission",
		// node:sqlite creates database files regardless of the permissions.
		"--no-experimental-sqlite",
		// V8 flags set at run time, such as those for traces, write files regardless of the permissions.
		"--freeze-flags-after-init",
		"--allow-fs-read="+inputPath,
		"--allow-fs-write="+outputPath,
		// Without a package.json Node.js would guess the module type of stdin.
		"--input-type=module",
		"-",
		"--input-file="+inputPath,
		"--output-file="+outputPath,
	)
	cmd.Dir = resolvedRenderDir
	cmd.Env = []string{}
	cmd.Stdin = bytes.NewReader(bundle)

	if runtime.GOOS == "windows" {
		cmd.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot")}
	}

	// Whether the chart fails is unknown until it exits, so stderr is logged as warnings as it comes,
	// and also goes into the error of a failed chart.
	var excerpt stderrExcerpt

	stdout := &chartOutputWriter{
		name:   "stdout",
		onLine: func(line string) { log.Default.Debug(ctx, "%s", line) },
		stop:   stop,
	}
	stderr := &chartOutputWriter{
		name: "stderr",
		onLine: func(line string) {
			log.Default.Warn(ctx, "%s", line)
			excerpt.add(line + "\n")
		},
		stop: stop,
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()

	stdout.flush()
	stderr.flush()

	switch {
	case stdout.exceeded:
		err = fmt.Errorf("the chart wrote more than %d MiB to stdout", maxChartOutputBytes>>20)
	case stderr.exceeded:
		err = fmt.Errorf("the chart wrote more than %d MiB to stderr", maxChartOutputBytes>>20)
	case runErr != nil:
		err = fmt.Errorf("run process: %w", runErr)
	default:
		return nil
	}

	if stderrText := excerpt.String(); stderrText != "" {
		return fmt.Errorf("%w:\n%s", err, stderrText)
	}

	return err
}
