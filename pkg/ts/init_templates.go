package ts

const (
	denoJSONTmpl = `{
  "tasks": {
    "build": "{{ .BuildScript }}"
  },
  "imports": {
    "@nelm/chart-ts-sdk": "npm:@nelm/chart-ts-sdk@^0.1.4"
  }
}
`
	deploymentTSTmpl = `import type { {{ .RenderContextType }} } from '@nelm/chart-ts-sdk';
import { getFullname, getLabels, getSelectorLabels } from './helpers.ts';

export function newDeployment($: {{ .RenderContextType }}): object {
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
              image: ` + "($.Values.image?.repository ?? 'nginx') + ':' + ($.Values.image?.tag ?? 'latest')" + `,
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
ts/vendor/
ts/node_modules/
`
	helpersTSTmpl = `import type { {{ .RenderContextType }} } from '@nelm/chart-ts-sdk';

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
export function getFullname($: {{ .RenderContextType }}): string {
  if ($.Values.fullnameOverride) {
    return trunc($.Values.fullnameOverride, 63);
  }

  const chartName = $.Values.nameOverride || $.Chart.Name;

  if ($.Release.Name.includes(chartName)) {
    return trunc($.Release.Name, 63);
  }

  return trunc(` + "`${$.Release.Name}-${chartName}`" + `, 63);
}

export function getLabels($: {{ .RenderContextType }}): Record<string, string> {
  return {
    'app.kubernetes.io/name': $.Chart.Name,
    'app.kubernetes.io/instance': $.Release.Name,
  };
}

export function getSelectorLabels($: {{ .RenderContextType }}): Record<string, string> {
  return {
    'app.kubernetes.io/name': $.Chart.Name,
    'app.kubernetes.io/instance': $.Release.Name,
  };
}
`
	indexTSTmpl = `import { {{ .RenderContextType }}, RenderResult, render } from '@nelm/chart-ts-sdk';
import { newDeployment } from './deployment.ts';
import { newService } from './service.ts';

function generate($: {{ .RenderContextType }}): RenderResult {
  const manifests: object[] = [];

  manifests.push(newDeployment($));

  if ($.Values.service?.enabled !== false) {
    manifests.push(newService($));
  }

  return { manifests };
}

await render(generate);
`
	inputExampleTmpl = `Capabilities:
  APIVersions:
    - v1
  HelmVersion:
    go_version: go1.25.0
    version: v3.20
  KubeVersion:
    Major: "1"
    Minor: "35"
    Version: v1.35.0
Chart:
  APIVersion: v2
  Annotations:
    anno: value
  AppVersion: 1.0.0
  Condition: {{ .ChartName }}.enabled
  Description: {{ .ChartName }} description
  Home: https://example.org/home
  Icon: https://example.org/icon
  Keywords:
    - {{ .ChartName }}
  Maintainers:
    - Email: john@example.com
      Name: john
      URL: https://example.com/john
  Name: {{ .ChartName }}
  Sources:
    - https://example.org/{{ .ChartName }}
  Tags: {{ .ChartName }}
  Type: application
  Version: 0.1.0
Files:
  myfile: "content"
Release:
  IsInstall: false
  IsUpgrade: true
  Name: {{ .ChartName }}
  Namespace: {{ .ChartName }}
  Revision: 2
  Service: Helm
Values:
  image:
    repository: nginx
    tag: latest
  replicaCount: 1
  service:
    enabled: true
    port: 80
    type: ClusterIP
`
	serviceTSTmpl = `import type { {{ .RenderContextType }} } from '@nelm/chart-ts-sdk';
import { getFullname, getLabels, getSelectorLabels } from './helpers.ts';

export function newService($: {{ .RenderContextType }}): object {
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
