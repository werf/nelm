package ts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const denoBuildScript = "deno bundle --output=dist/bundle.js src/index.ts"

type InitTSBoilerplateOptions struct {
	RenderContextType string
}

type initTmplData struct {
	BuildScript       string
	ChartName         string
	RenderContextType string
}

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

func InitTSBoilerplate(ctx context.Context, chartPath, chartName string, opts InitTSBoilerplateOptions) error {
	tsDir := filepath.Join(chartPath, common.ChartTSSourceDir)
	srcDir := filepath.Join(tsDir, "src")

	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("init ts boilerplate: typescript directory already exists: %s", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	ctxType := common.TSDefaultRenderContextType
	if opts.RenderContextType != "" {
		ctxType = opts.RenderContextType
	}

	data := initTmplData{
		BuildScript:       denoBuildScript,
		ChartName:         chartName,
		RenderContextType: ctxType,
	}

	files := []struct {
		tmpl string
		path string
	}{
		{tmpl: indexTSTmpl, path: filepath.Join(srcDir, "index.ts")},
		{tmpl: helpersTSTmpl, path: filepath.Join(srcDir, "helpers.ts")},
		{tmpl: deploymentTSTmpl, path: filepath.Join(srcDir, "deployment.ts")},
		{tmpl: serviceTSTmpl, path: filepath.Join(srcDir, "service.ts")},
		{tmpl: tsconfigContent, path: filepath.Join(tsDir, "tsconfig.json")},
		{tmpl: denoJSONTmpl, path: filepath.Join(tsDir, "deno.json")},
		{tmpl: inputExampleTmpl, path: filepath.Join(tsDir, "input.example.yaml")},
	}

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", srcDir, err)
	}

	for _, f := range files {
		content, err := renderTemplate(f.tmpl, data)
		if err != nil {
			return fmt.Errorf("render template for %s: %w", f.path, err)
		}

		if err := os.WriteFile(f.path, []byte(content), 0o644); err != nil {
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

func renderTemplate(tmplStr string, data initTmplData) (string, error) {
	t, err := template.New("").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
