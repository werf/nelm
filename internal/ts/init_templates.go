package ts

import "fmt"

const (
	chartYamlTmpl = `apiVersion: v2
name: %s
version: 0.1.0
`
	deploymentTSContent = `import { RenderContext } from '@nelm/types';
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
	helmignoreContent = `# Patterns to ignore when building packages.
# This supports shell glob matching, relative path matching, and
# negation (prefixed with !). Only one pattern per line.

# TypeScript chart files
ts/dist/
`
	helpersTSContent = `import { RenderContext } from '@nelm/types';

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
	indexTSContent = `import { RenderContext, RenderResult } from '@nelm/types';
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
	packageJSONTmpl = `{
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
`
	serviceTSContent = `import { RenderContext } from '@nelm/types';
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
	tsconfigContent = `{
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
	valuesYamlContent = `replicaCount: 1

image:
  repository: nginx
  tag: latest

service:
  enabled: true
  type: ClusterIP
  port: 80
`
)

func chartYaml(chartName string) string {
	return fmt.Sprintf(chartYamlTmpl, chartName)
}

func packageJSON(chartName string) string {
	return fmt.Sprintf(packageJSONTmpl, chartName, chartName)
}
