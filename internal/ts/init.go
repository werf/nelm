package ts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tsbundle "github.com/werf/3p-helm/pkg/werf/ts"
	"github.com/werf/nelm/pkg/log"
)

const denoBuildScript = "deno bundle --output=dist/bundle.js src/index.ts"

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

// InitChartStructure creates Chart.yaml and values.yaml if they don't exist.
// For .helmignore: creates if missing, or appends TS entries if exists.
// Returns error if ts/ directory already exists.
func InitChartStructure(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, tsbundle.ChartTSSourceDir)
	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("init chart structure: typescript directory already exists: %s", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	skipIfExists := []struct {
		content string
		path    string
	}{
		{content: chartYaml(chartName), path: filepath.Join(chartPath, "Chart.yaml")},
		{content: valuesYamlContent, path: filepath.Join(chartPath, "values.yaml")},
	}

	for _, f := range skipIfExists {
		_, err := os.Stat(f.path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", f.path, err)
		}

		if err == nil {
			log.Default.Debug(ctx, "Skipping existing file %s", f.path)
			continue
		}

		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}

		log.Default.Debug(ctx, "Created %s", f.path)
	}

	// Handle .helmignore: create or enrich
	helmignorePath := filepath.Join(chartPath, ".helmignore")
	if err := ensureFileEntries(helmignorePath, helmignoreContent, []string{"ts/vendor/", "ts/node_modules/"}); err != nil {
		return fmt.Errorf("ensure helmignore entries: %w", err)
	}

	log.Default.Debug(ctx, "Updated %s", helmignorePath)

	return nil
}

// InitTSBoilerplate creates TypeScript boilerplate files in ts/ directory.
func InitTSBoilerplate(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, tsbundle.ChartTSSourceDir)
	srcDir := filepath.Join(tsDir, "src")

	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("init ts boilerplate: typescript directory already exists: %s", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	files := []struct {
		content string
		path    string
	}{
		{content: indexTSContent, path: filepath.Join(srcDir, "index.ts")},
		{content: helpersTSContent, path: filepath.Join(srcDir, "helpers.ts")},
		{content: deploymentTSContent, path: filepath.Join(srcDir, "deployment.ts")},
		{content: serviceTSContent, path: filepath.Join(srcDir, "service.ts")},
		{content: tsconfigContent, path: filepath.Join(tsDir, "tsconfig.json")},
		{content: denoJSON(denoBuildScript), path: filepath.Join(tsDir, "deno.json")},
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
