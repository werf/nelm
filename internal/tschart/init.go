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
func InitChartStructure(ctx context.Context, chartPath, chartName string) error {
	skipIfExists := []struct {
		path    string
		content string
	}{
		{filepath.Join(chartPath, "Chart.yaml"), generateChartYaml(chartName)},
		{filepath.Join(chartPath, "values.yaml"), generateValuesYaml()},
	}

	for _, f := range skipIfExists {
		if _, err := os.Stat(f.path); err == nil {
			log.Default.Debug(ctx, "Skipping existing file %s", f.path)
			continue
		}

		if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		log.Default.Debug(ctx, "Created %s", f.path)
	}

	// Handle .helmignore specially: create or enrich
	helmignorePath := filepath.Join(chartPath, ".helmignore")
	if _, err := os.Stat(helmignorePath); err == nil {
		if err := AppendToHelmignore(chartPath); err != nil {
			return fmt.Errorf("enrich .helmignore: %w", err)
		}
		log.Default.Debug(ctx, "Enriched existing %s with TypeScript entries", helmignorePath)
	} else {
		if err := os.WriteFile(helmignorePath, []byte(generateHelmignoreWithTS()), 0644); err != nil {
			return fmt.Errorf("write %s: %w", helmignorePath, err)
		}
		log.Default.Debug(ctx, "Created %s", helmignorePath)
	}

	return nil
}

// InitTSBoilerplate creates TypeScript boilerplate files in ts/ directory.
// Returns error if any TypeScript file already exists.
func InitTSBoilerplate(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, TSSourceDir)
	srcDir := filepath.Join(tsDir, "src")
	typesDir := filepath.Join(tsDir, "types")

	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(srcDir, "index.ts"), generateIndexTS()},
		{filepath.Join(srcDir, "helpers.ts"), generateHelpersTS()},
		{filepath.Join(srcDir, "resources.ts"), generateResourcesTS()},
		{filepath.Join(typesDir, "nelm.d.ts"), generateNelmDTS()},
		{filepath.Join(tsDir, "tsconfig.json"), generateTSConfig()},
		{filepath.Join(tsDir, "package.json"), generatePackageJSON(chartName)},
		{filepath.Join(tsDir, ".gitignore"), generateTSGitignore()},
	}

	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			return fmt.Errorf("TypeScript file already exists: %s. Cannot initialize in a directory with existing TypeScript chart files", f.path)
		}
	}

	for _, dir := range []string{srcDir, typesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
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
	if strings.Contains(content, "ts/node_modules/") {
		return nil
	}

	tsEntries := `
# TypeScript chart files
ts/node_modules/
ts/dist/
`
	newContent := strings.TrimRight(content, "\n") + "\n" + tsEntries

	if err := os.WriteFile(helmignorePath, []byte(newContent), 0644); err != nil {
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
		if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
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
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}

	return nil
}

func generateIndexTS() string {
	return `import { RenderContext, RenderResult } from '../types/nelm';
import { newDeployment, newService } from './resources';

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
	return `import { RenderContext } from '../types/nelm';

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
export function fullname($: RenderContext): string {
  if ($.Values.fullnameOverride) {
    return trunc($.Values.fullnameOverride, 63);
  }

  const chartName = $.Values.nameOverride || $.Chart.Name;

  if ($.Release.Name.includes(chartName)) {
    return trunc($.Release.Name, 63);
  }

  return trunc(` + "`${$.Release.Name}-${chartName}`" + `, 63);
}

export function labels($: RenderContext): Record<string, string> {
  return {
    'app.kubernetes.io/name': $.Chart.Name,
    'app.kubernetes.io/instance': $.Release.Name,
  };
}

export function selectorLabels($: RenderContext): Record<string, string> {
  return {
    'app.kubernetes.io/name': $.Chart.Name,
    'app.kubernetes.io/instance': $.Release.Name,
  };
}
`
}

func generateResourcesTS() string {
	return `import { RenderContext } from '../types/nelm';
import { fullname, labels, selectorLabels } from './helpers';

export function newDeployment($: RenderContext): object {
  const name = fullname($);

  return {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: {
      name,
      labels: labels($),
    },
    spec: {
      replicas: $.Values.replicaCount ?? 1,
      selector: {
        matchLabels: selectorLabels($),
      },
      template: {
        metadata: {
          labels: selectorLabels($),
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

export function newService($: RenderContext): object {
  return {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: {
      name: fullname($),
      labels: labels($),
    },
    spec: {
      type: $.Values.service?.type ?? 'ClusterIP',
      ports: [
        {
          port: $.Values.service?.port ?? 80,
          targetPort: 'http',
        },
      ],
      selector: selectorLabels($),
    },
  };
}
`
}

func generateNelmDTS() string {
	return `export interface RenderContext {
  Values: Record<string, any>;
  Release: Release;
  Chart: ChartMetadata;
  Capabilities: Capabilities;
  Files: Record<string, Uint8Array>;
}

export interface Release {
  Name: string;
  Namespace: string;
  Revision: number;
  IsInstall: boolean;
  IsUpgrade: boolean;
  Service: string;
}

export interface ChartMetadata {
  Name: string;
  Version: string;
  AppVersion: string;
}

export interface Capabilities {
  APIVersions: string[];
  KubeVersion: { Version: string; Major: string; Minor: string };
}

export interface RenderResult {
  manifests: object[] | null;
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
  "devDependencies": {
    "typescript": "^5.0.0"
  }
}
`, chartName, chartName)
}

func generateTSGitignore() string {
	return `# Dependencies
node_modules/

# Vendor bundle (generated by nelm chart pack)
vendor/

# TypeScript build output
dist/
`
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
ts/node_modules/
ts/dist/
`
}
