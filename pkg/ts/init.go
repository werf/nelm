package ts

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/tidwall/sjson"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/log"
)

var chartTSIgnoreEntries = []string{
	"ts/node_modules/",
	"ts/.dev/",
	"ts/.pnp.*",
	"ts/.yarn/cache/",
	"ts/.yarn/unplugged/",
	"ts/.yarn/__virtual__/",
	"ts/.yarn/install-state.gz",
	"ts/.yarn/build-state.yml",
}

type InitTSBoilerplateOptions struct {
	RenderContextType string
}

// EnsureGitignore adds TypeScript entries to .gitignore, creating if needed.
func EnsureGitignore(chartPath string) error {
	return ensureFileEntries(
		filepath.Join(chartPath, ".gitignore"),
		strings.Join(chartTSIgnoreEntries, "\n")+"\n",
		chartTSIgnoreEntries,
	)
}

// InitChartStructure creates values.yaml and .helmignore if they don't exist.
// If values.yaml already exists, creates values-ts-example.yaml instead.
// Returns error if ts/ directory already exists.
func InitChartStructure(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, common.ChartTSSourceDir)
	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("init chart structure: typescript directory already exists: %s: remove it to initialize the chart again, or, if installing the dependencies failed when it was initialized, install them by running the install command of the package manager there, such as \"npm install\"", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	if err := ensureValuesFile(ctx, chartPath); err != nil {
		return fmt.Errorf("ensure values.yaml: %w", err)
	}

	// Handle .helmignore: create or enrich
	helmignorePath := filepath.Join(chartPath, ".helmignore")
	if err := ensureFileEntries(helmignorePath, helmignoreContent, chartTSIgnoreEntries); err != nil {
		return fmt.Errorf("ensure helmignore entries: %w", err)
	}

	log.Default.Debug(ctx, "Updated %s", helmignorePath)

	return nil
}

func InitTSBoilerplate(ctx context.Context, chartPath, chartName string, opts InitTSBoilerplateOptions) error {
	tsDir := filepath.Join(chartPath, common.ChartTSSourceDir)

	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("init ts boilerplate: typescript directory already exists: %s", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	ctxType := common.TSDefaultRenderContextType
	if opts.RenderContextType != "" {
		ctxType = opts.RenderContextType
	}

	if err := fs.WalkDir(scaffoldFiles, "scaffold", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk scaffold: %w", err)
		}

		if entry.IsDir() {
			return nil
		}

		content, err := scaffoldFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read scaffold file %s: %w", path, err)
		}

		relPath := strings.TrimPrefix(path, "scaffold/")
		switch relPath {
		case "package.json":
			content, err = sjson.SetBytes(content, "name", strings.ToLower(chartName)+"-ts")
		case "src/context.ts":
			content = []byte(fmt.Sprintf("export type { %s as ChartContext } from '@nelm/chart-ts-sdk';\n", ctxType))
		case "input.example.yaml":
			content, err = scaffoldInput(content, chartName, ctxType)
		case "src/deployment.test.ts":
			if ctxType == common.TSWerfRenderContextType {
				extraValues, err := yaml.YAMLToJSON([]byte(scaffoldExtraValues))
				if err != nil {
					return fmt.Errorf("convert extra test values to JSON: %w", err)
				}

				testValues, err := json.MarshalIndent(map[string]json.RawMessage{"global": extraValues}, "", "  ")
				if err != nil {
					return fmt.Errorf("format extra test values: %w", err)
				}

				testValues = formatScaffoldTestValues(testValues)
				content = []byte(strings.Replace(string(content), "const defaultValues = {};", fmt.Sprintf("const defaultValues = %s;", testValues), 1))
			}
		}

		if err != nil {
			return fmt.Errorf("customize scaffold file %s: %w", relPath, err)
		}

		destPath := filepath.Join(tsDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", destPath, err)
		}

		if err := os.WriteFile(destPath, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", destPath, err)
		}

		log.Default.Debug(ctx, "Created %s", destPath)

		return nil
	}); err != nil {
		return fmt.Errorf("copy scaffold: %w", err)
	}

	return nil
}

func formatScaffoldTestValues(content []byte) []byte {
	return []byte(regexp.MustCompile(`"[A-Za-z_$][A-Za-z0-9_$]*":|"(?:[^"\\]|\\.)*"`).ReplaceAllStringFunc(string(content), func(value string) string {
		if strings.HasSuffix(value, ":") {
			return value[1:len(value)-2] + ":"
		}

		value = strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)

		return "'" + strings.ReplaceAll(value, "'", `\'`) + "'"
	}))
}

func ensureValuesFile(ctx context.Context, chartPath string) error {
	valuesPath := filepath.Join(chartPath, "values.yaml")

	exists, err := fileExists(valuesPath)
	if err != nil {
		return err
	}

	if !exists {
		if err := os.WriteFile(valuesPath, []byte(valuesYamlContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", valuesPath, err)
		}

		log.Default.Debug(ctx, "Created %s", valuesPath)

		return nil
	}

	examplePath := filepath.Join(chartPath, "values-ts-example.yaml")

	exists, err = fileExists(examplePath)
	if err != nil {
		return err
	}

	if exists {
		log.Default.Debug(ctx, "Skipping existing file %s", examplePath)

		return nil
	}

	if err := os.WriteFile(examplePath, []byte(valuesYamlContent), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", examplePath, err)
	}

	log.Default.Warn(ctx, "values.yaml already exists, created values-ts-example.yaml instead")

	return nil
}

// ensureFileEntries ensures a file contains all required entries.
// If file doesn't exist, creates it with defaultContent.
// If file exists, appends any missing entries.
func ensureFileEntries(filePath, defaultContent string, requiredEntries []string) error {
	existingContent, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		if err := os.WriteFile(filePath, []byte(defaultContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", filePath, err)
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	content := string(existingContent)

	var toAdd []string
	for _, entry := range requiredEntries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	newContent := strings.TrimRight(content, "\n") + "\n\n# TypeScript chart files\n" + strings.Join(toAdd, "\n") + "\n"

	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil { //nolint:gosec // G703: filePath is inside the chart directory the user asked to initialize
		return fmt.Errorf("write %s: %w", filePath, err)
	}

	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, fmt.Errorf("stat %s: %w", path, err)
}

func scaffoldInput(content []byte, chartName, contextType string) ([]byte, error) {
	content, err := yaml.YAMLToJSON(content)
	if err != nil {
		return nil, fmt.Errorf("convert input example to JSON: %w", err)
	}

	for _, replacement := range []struct {
		path  string
		value string
	}{
		{path: "Chart.Condition", value: chartName + ".enabled"},
		{path: "Chart.Description", value: chartName + " description"},
		{path: "Chart.Keywords.0", value: chartName},
		{path: "Chart.Name", value: chartName},
		{path: "Chart.Sources.0", value: "https://example.org/" + chartName},
		{path: "Chart.Tags", value: chartName},
		{path: "Release.Name", value: chartName},
		{path: "Release.Namespace", value: chartName},
	} {
		content, err = sjson.SetBytes(content, replacement.path, replacement.value)
		if err != nil {
			return nil, fmt.Errorf("replace %s: %w", replacement.path, err)
		}
	}

	if contextType == common.TSWerfRenderContextType {
		extraValues, err := yaml.YAMLToJSON([]byte(scaffoldExtraValues))
		if err != nil {
			return nil, fmt.Errorf("convert extra values to JSON: %w", err)
		}

		if content, err = sjson.SetRawBytes(content, "Values.global", extraValues); err != nil {
			return nil, fmt.Errorf("set extra values: %w", err)
		}
	}

	content, err = yaml.JSONToYAML(content)
	if err != nil {
		return nil, fmt.Errorf("convert input example to YAML: %w", err)
	}

	return content, nil
}
