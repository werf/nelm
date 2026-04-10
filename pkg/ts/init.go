package ts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	denoBuildScript          = "deno bundle --output=dist/bundle.js src/index.ts"
	defaultRenderContextType = "RenderContext"
)

// EnsureGitignore adds TypeScript entries to .gitignore, creating if needed.
func EnsureGitignore(chartPath string) error {
	entries := []string{
		"ts/node_modules/",
		"ts/vendor/",
	}

	return ensureFileEntries(
		filepath.Join(chartPath, ".gitignore"),
		strings.Join(entries, "\n")+"\n",
		entries,
	)
}

// InitChartStructure creates values.yaml and .helmignore if they don't exist.
// If values.yaml already exists, creates values-ts-example.yaml instead.
// Returns error if ts/ directory already exists.
func InitChartStructure(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, common.ChartTSSourceDir)
	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("init chart structure: typescript directory already exists: %s", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	if err := ensureValuesFile(ctx, chartPath); err != nil {
		return fmt.Errorf("ensure values.yaml: %w", err)
	}

	// Handle .helmignore: create or enrich
	helmignorePath := filepath.Join(chartPath, ".helmignore")
	if err := ensureFileEntries(helmignorePath, helmignoreContent, []string{"ts/vendor/", "ts/node_modules/"}); err != nil {
		return fmt.Errorf("ensure helmignore entries: %w", err)
	}

	log.Default.Debug(ctx, "Updated %s", helmignorePath)

	return nil
}

type InitTSBoilerplateOptions struct {
	RenderContextType string
}

func InitTSBoilerplate(ctx context.Context, chartPath, chartName string, opts InitTSBoilerplateOptions) error {
	tsDir := filepath.Join(chartPath, common.ChartTSSourceDir)
	srcDir := filepath.Join(tsDir, "src")

	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("init ts boilerplate: typescript directory already exists: %s", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	ctxType := defaultRenderContextType
	if opts.RenderContextType != "" {
		ctxType = opts.RenderContextType
	}

	files := []struct {
		content string
		path    string
	}{
		{content: strings.ReplaceAll(indexTSTmpl, renderContextTypePlaceholder, ctxType), path: filepath.Join(srcDir, "index.ts")},
		{content: strings.ReplaceAll(helpersTSTmpl, renderContextTypePlaceholder, ctxType), path: filepath.Join(srcDir, "helpers.ts")},
		{content: strings.ReplaceAll(deploymentTSTmpl, renderContextTypePlaceholder, ctxType), path: filepath.Join(srcDir, "deployment.ts")},
		{content: strings.ReplaceAll(serviceTSTmpl, renderContextTypePlaceholder, ctxType), path: filepath.Join(srcDir, "service.ts")},
		{content: tsconfigContent, path: filepath.Join(tsDir, "tsconfig.json")},
		{content: fmt.Sprintf(denoJSONTmpl, denoBuildScript), path: filepath.Join(tsDir, "deno.json")},
		{content: fmt.Sprintf(inputExampleContent, chartName), path: filepath.Join(tsDir, "input.example.yaml")},
	}

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", srcDir, err)
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}

		log.Default.Debug(ctx, "Created %s", f.path)
	}

	return nil
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

	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
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
