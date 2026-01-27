package tschart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/werf/nelm/pkg/log"
)

// InitChartStructure creates Chart.yaml and values.yaml if they don't exist.
// For .helmignore: creates if missing, or appends TS entries if exists.
// Returns error if ts/ directory already exists.
func InitChartStructure(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, TSSourceDir)
	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("TypeScript directory already exists: %s. Cannot initialize in a directory with existing TypeScript chart files", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	skipIfExists := []struct {
		path    string
		content string
	}{
		{filepath.Join(chartPath, "Chart.yaml"), generateChartYaml(chartName)},
		{filepath.Join(chartPath, "values.yaml"), generateValuesYaml()},
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

		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil { //nolint:gosec // Chart files should be world-readable
			return fmt.Errorf("write %s: %w", f.path, err)
		}

		log.Default.Debug(ctx, "Created %s", f.path)
	}

	// Handle .helmignore specially: create or enrich
	helmignorePath := filepath.Join(chartPath, ".helmignore")

	_, err := os.Stat(helmignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", helmignorePath, err)
	}

	if err == nil {
		if err := AppendToHelmignore(chartPath); err != nil {
			return fmt.Errorf("enrich .helmignore: %w", err)
		}

		log.Default.Debug(ctx, "Enriched existing %s with TypeScript entries", helmignorePath)
	} else {
		if err := os.WriteFile(helmignorePath, []byte(generateHelmignoreWithTS()), 0o644); err != nil { //nolint:gosec // Chart files should be world-readable
			return fmt.Errorf("write %s: %w", helmignorePath, err)
		}

		log.Default.Debug(ctx, "Created %s", helmignorePath)
	}

	return nil
}

// InitTSBoilerplate creates TypeScript boilerplate files in ts/ directory.
func InitTSBoilerplate(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, TSSourceDir)
	srcDir := filepath.Join(tsDir, "src")

	if _, err := os.Stat(tsDir); err == nil {
		return fmt.Errorf("TypeScript directory already exists: %s", tsDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(srcDir, "index.ts"), generateIndexTS()},
		{filepath.Join(srcDir, "helpers.ts"), generateHelpersTS()},
		{filepath.Join(srcDir, "deployment.ts"), generateDeploymentTS()},
		{filepath.Join(srcDir, "service.ts"), generateServiceTS()},
		{filepath.Join(tsDir, "tsconfig.json"), generateTSConfig()},
		{filepath.Join(tsDir, "package.json"), generatePackageJSON(chartName)},
	}

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", srcDir, err)
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil { //nolint:gosec // Chart files should be world-readable
			return fmt.Errorf("write %s: %w", f.path, err)
		}

		log.Default.Debug(ctx, "Created %s", f.path)
	}

	return nil
}

func AppendToHelmignore(chartPath string) error {
	helmignorePath := filepath.Join(chartPath, ".helmignore")

	existingContent, err := os.ReadFile(helmignorePath)
	if err != nil {
		return fmt.Errorf("read .helmignore: %w", err)
	}

	content := string(existingContent)
	if strings.Contains(content, "ts/dist/") {
		return nil
	}

	tsEntries := `
# TypeScript chart files
ts/dist/
`
	newContent := strings.TrimRight(content, "\n") + "\n" + tsEntries

	if err := os.WriteFile(helmignorePath, []byte(newContent), 0o644); err != nil { //nolint:gosec // Chart files should be world-readable
		return fmt.Errorf("write .helmignore: %w", err)
	}

	return nil
}

func EnsureGitignore(chartPath string) error {
	gitignorePath := filepath.Join(chartPath, ".gitignore")

	entries := []string{
		"ts/node_modules/",
		"ts/vendor/",
		"ts/dist/",
	}

	existingContent, err := os.ReadFile(gitignorePath)
	if os.IsNotExist(err) {
		content := strings.Join(entries, "\n") + "\n"
		if err := os.WriteFile(gitignorePath, []byte(content), 0o644); err != nil { //nolint:gosec // Chart files should be world-readable
			return fmt.Errorf("create .gitignore: %w", err)
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	content := string(existingContent)

	var toAdd []string

	for _, entry := range entries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	newContent := strings.TrimRight(content, "\n") + "\n" + strings.Join(toAdd, "\n") + "\n"
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil { //nolint:gosec // Chart files should be world-readable
		return fmt.Errorf("write .gitignore: %w", err)
	}

	return nil
}

func generateIndexTS() string {
	return `import { RenderContext, RenderResult } from '@nelm/types';
import { newDeployment } from './deployment';
import { newService } from './service';

export function render($: RenderContext): RenderResult {
  const manifests: object[] = [];

  manifests.push(newDeployment($));

  if ($.Values.service?.enabled !== false) {
    manifests.push(newService($));
  }

  return { manifests };
}
`
}

func generateHelpersTS() string {
	return `import { RenderContext } from '@nelm/types';

/**
 * Truncate string to max length, removing trailing hyphens.
 */
export function trunc(str: string, max: number): string {
  if (str.length <= max) return str;
  return str.slice(0, max).replace(/-+$/, '');
}

/**
 * Get the fully qualified app name.
 * Truncated at 63 chars (DNS naming spec limit).
 */
export function getFullname($: RenderContext): string {
  if ($.Values.fullnameOverride) {
    return trunc($.Values.fullnameOverride, 63);
  }

  const chartName = $.Values.nameOverride || $.Chart.Name;

  if ($.Release.Name.includes(chartName)) {
    return trunc($.Release.Name, 63);
  }

  return trunc(` + "`${$.Release.Name}-${chartName}`" + `, 63);
}

export function getLabels($: RenderContext): Record<string, string> {
  return {
    'app.kubernetes.io/name': $.Chart.Name,
    'app.kubernetes.io/instance': $.Release.Name,
  };
}

export function getSelectorLabels($: RenderContext): Record<string, string> {
  return {
    'app.kubernetes.io/name': $.Chart.Name,
    'app.kubernetes.io/instance': $.Release.Name,
  };
}
`
}

func generateDeploymentTS() string {
	return `import { RenderContext } from '@nelm/types';
import { getFullname, getLabels, getSelectorLabels } from './helpers';

export function newDeployment($: RenderContext): object {
  const name = getFullname($);

  return {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: {
      name,
      labels: getLabels($),
    },
    spec: {
      replicas: $.Values.replicaCount ?? 1,
      selector: {
        matchLabels: getSelectorLabels($),
      },
      template: {
        metadata: {
          labels: getSelectorLabels($),
        },
        spec: {
          containers: [
            {
              name: name,
              image: ` + "`${$.Values.image?.repository}:${$.Values.image?.tag}`" + `,
              ports: [
                {
                  name: 'http',
                  containerPort: $.Values.service?.port ?? 80,
                },
              ],
            },
          ],
        },
      },
    },
  };
}
`
}

func generateServiceTS() string {
	return `import { RenderContext } from '@nelm/types';
import { getFullname, getLabels, getSelectorLabels } from './helpers';

export function newService($: RenderContext): object {
  return {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: {
      name: getFullname($),
      labels: getLabels($),
    },
    spec: {
      type: $.Values.service?.type ?? 'ClusterIP',
      ports: [
        {
          port: $.Values.service?.port ?? 80,
          targetPort: 'http',
        },
      ],
      selector: getSelectorLabels($),
    },
  };
}
`
}

func generateTSConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2015",
    "module": "CommonJS",
    "declaration": true,
    "declarationMap": true,
    "inlineSourceMap": true,
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "moduleResolution": "node",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "outDir": "./dist"
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
`
}

func generatePackageJSON(chartName string) string {
	return fmt.Sprintf(`{
  "name": "%s",
  "version": "0.1.0",
  "description": "TypeScript chart for %s",
  "main": "src/index.ts",
  "scripts": {
    "build": "npx tsc --noEmit",
    "typecheck": "npx tsc --noEmit"
  },
  "keywords": [
    "helm",
    "nelm",
    "kubernetes",
    "chart"
  ],
  "license": "Apache-2.0",
  "dependencies": {
    "@nelm/types": "^0.1.0"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  }
}
`, chartName, chartName)
}

func generateChartYaml(chartName string) string {
	return fmt.Sprintf(`apiVersion: v2
name: %s
version: 0.1.0
`, chartName)
}

func generateValuesYaml() string {
	return `replicaCount: 1

image:
  repository: nginx
  tag: latest

service:
  enabled: true
  type: ClusterIP
  port: 80
`
}

func generateHelmignoreWithTS() string {
	return `# Patterns to ignore when building packages.
# This supports shell glob matching, relative path matching, and
# negation (prefixed with !). Only one pattern per line.

# TypeScript chart files
ts/dist/
`
}
