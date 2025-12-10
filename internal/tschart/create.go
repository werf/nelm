package tschart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/werf/nelm/pkg/log"
)

func CreateTSBoilerplate(ctx context.Context, chartPath, chartName string) error {
	tsDir := filepath.Join(chartPath, TSSourceDir)
	srcDir := filepath.Join(tsDir, "src")
	typesDir := filepath.Join(tsDir, "types")

	for _, dir := range []string{srcDir, typesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	files := map[string]string{
		filepath.Join(srcDir, "index.ts"):     generateIndexTS(),
		filepath.Join(srcDir, "helpers.ts"):   generateHelpersTS(),
		filepath.Join(srcDir, "resources.ts"): generateResourcesTS(),
		filepath.Join(typesDir, "nelm.d.ts"):  generateNelmDTS(),
		filepath.Join(tsDir, "tsconfig.json"): generateTSConfig(),
		filepath.Join(tsDir, "package.json"):  generatePackageJSON(chartName),
		filepath.Join(tsDir, ".gitignore"):    generateTSGitignore(),
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		log.Default.Debug(ctx, "Created %s", path)
	}

	return nil
}

func CreateTSOnlyChartStructure(ctx context.Context, chartPath, chartName string) error {
	chartsDir := filepath.Join(chartPath, "charts")
	if err := os.MkdirAll(chartsDir, 0755); err != nil {
		return fmt.Errorf("create charts directory: %w", err)
	}

	files := map[string]string{
		filepath.Join(chartPath, "Chart.yaml"):  generateChartYaml(chartName),
		filepath.Join(chartPath, "values.yaml"): generateValuesYaml(chartName),
		filepath.Join(chartPath, ".helmignore"): generateHelmignoreWithTS(),
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		log.Default.Debug(ctx, "Created %s", path)
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

func generateIndexTS() string {
	return `import { RenderContext, RenderResult } from '../types/nelm';
import {
  createDeployment,
  createService,
  createServiceAccount,
  createIngress,
  createHPA,
} from './resources';
import {
  shouldCreateIngress,
  shouldCreateHPA,
  shouldCreateServiceAccount,
} from './helpers';

export function render(context: RenderContext): RenderResult {
  const manifests: object[] = [];

  // ServiceAccount
  if (shouldCreateServiceAccount(context)) {
    manifests.push(createServiceAccount(context));
  }

  // Deployment
  manifests.push(createDeployment(context));

  // Service
  manifests.push(createService(context));

  // Ingress
  if (shouldCreateIngress(context)) {
    manifests.push(createIngress(context));
  }

  // HorizontalPodAutoscaler
  if (shouldCreateHPA(context)) {
    manifests.push(createHPA(context));
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
 * Get the chart name, respecting nameOverride.
 */
export function name(context: RenderContext): string {
  const override = context.Values.nameOverride;
  return trunc(override || context.Chart.Name, 63);
}

/**
 * Get the fully qualified app name.
 * Truncated at 63 chars (DNS naming spec limit).
 */
export function fullname(context: RenderContext): string {
  const { Release, Chart, Values } = context;

  if (Values.fullnameOverride) {
    return trunc(Values.fullnameOverride, 63);
  }

  const chartName = Values.nameOverride || Chart.Name;

  if (Release.Name.includes(chartName)) {
    return trunc(Release.Name, 63);
  }

  return trunc(` + "`${Release.Name}-${chartName}`" + `, 63);
}

/**
 * Get chart name and version for chart label.
 */
export function chart(context: RenderContext): string {
  const str = ` + "`${context.Chart.Name}-${context.Chart.Version}`" + `.replace(/\\+/g, '_');
  return trunc(str, 63);
}

/**
 * Common labels for all resources.
 */
export function labels(context: RenderContext): Record<string, string> {
  return {
    'helm.sh/chart': chart(context),
    ...selectorLabels(context),
    ...(context.Chart.AppVersion
      ? { 'app.kubernetes.io/version': context.Chart.AppVersion }
      : {}),
    'app.kubernetes.io/managed-by': context.Release.Service,
  };
}

/**
 * Selector labels for matching pods.
 */
export function selectorLabels(context: RenderContext): Record<string, string> {
  return {
    'app.kubernetes.io/name': name(context),
    'app.kubernetes.io/instance': context.Release.Name,
  };
}

/**
 * Get the service account name.
 */
export function serviceAccountName(context: RenderContext): string {
  const sa = context.Values.serviceAccount;
  if (sa?.create) {
    return sa.name || fullname(context);
  }
  return sa?.name || 'default';
}

/**
 * Check if ServiceAccount should be created.
 */
export function shouldCreateServiceAccount(context: RenderContext): boolean {
  return context.Values.serviceAccount?.create === true;
}

/**
 * Check if Ingress should be created.
 */
export function shouldCreateIngress(context: RenderContext): boolean {
  return context.Values.ingress?.enabled === true;
}

/**
 * Check if HPA should be created.
 */
export function shouldCreateHPA(context: RenderContext): boolean {
  return context.Values.autoscaling?.enabled === true;
}
`
}

func generateResourcesTS() string {
	return `import { RenderContext } from '../types/nelm';
import { fullname, labels, selectorLabels, serviceAccountName } from './helpers';

/**
 * Create a Deployment resource.
 */
export function createDeployment(context: RenderContext): object {
  const { Values, Chart } = context;
  const name = fullname(context);

  return {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: {
      name,
      labels: labels(context),
    },
    spec: {
      ...(Values.autoscaling?.enabled
        ? {}
        : { replicas: Values.replicaCount ?? 1 }),
      selector: {
        matchLabels: selectorLabels(context),
      },
      template: {
        metadata: {
          labels: {
            ...selectorLabels(context),
            ...Values.podLabels,
          },
          ...(Values.podAnnotations
            ? { annotations: Values.podAnnotations }
            : {}),
        },
        spec: {
          ...(Values.imagePullSecrets?.length
            ? { imagePullSecrets: Values.imagePullSecrets }
            : {}),
          serviceAccountName: serviceAccountName(context),
          ...(Values.podSecurityContext
            ? { securityContext: Values.podSecurityContext }
            : {}),
          containers: [
            {
              name: Chart.Name,
              ...(Values.securityContext
                ? { securityContext: Values.securityContext }
                : {}),
              image: ` + "`${Values.image?.repository}:${Values.image?.tag || Chart.AppVersion}`" + `,
              imagePullPolicy: Values.image?.pullPolicy ?? 'IfNotPresent',
              ports: [
                {
                  name: 'http',
                  containerPort: Values.service?.port ?? 80,
                  protocol: 'TCP',
                },
              ],
              ...(Values.livenessProbe
                ? { livenessProbe: Values.livenessProbe }
                : {}),
              ...(Values.readinessProbe
                ? { readinessProbe: Values.readinessProbe }
                : {}),
              ...(Values.resources ? { resources: Values.resources } : {}),
              ...(Values.volumeMounts?.length
                ? { volumeMounts: Values.volumeMounts }
                : {}),
            },
          ],
          ...(Values.volumes?.length ? { volumes: Values.volumes } : {}),
          ...(Values.nodeSelector ? { nodeSelector: Values.nodeSelector } : {}),
          ...(Values.affinity ? { affinity: Values.affinity } : {}),
          ...(Values.tolerations?.length
            ? { tolerations: Values.tolerations }
            : {}),
        },
      },
    },
  };
}

/**
 * Create a Service resource.
 */
export function createService(context: RenderContext): object {
  const { Values } = context;

  return {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: {
      name: fullname(context),
      labels: labels(context),
    },
    spec: {
      type: Values.service?.type ?? 'ClusterIP',
      ports: [
        {
          port: Values.service?.port ?? 80,
          targetPort: 'http',
          protocol: 'TCP',
          name: 'http',
        },
      ],
      selector: selectorLabels(context),
    },
  };
}

/**
 * Create a ServiceAccount resource.
 */
export function createServiceAccount(context: RenderContext): object {
  const { Values } = context;

  return {
    apiVersion: 'v1',
    kind: 'ServiceAccount',
    metadata: {
      name: serviceAccountName(context),
      labels: labels(context),
      ...(Values.serviceAccount?.annotations
        ? { annotations: Values.serviceAccount.annotations }
        : {}),
    },
    automountServiceAccountToken: Values.serviceAccount?.automount ?? true,
  };
}

/**
 * Create an Ingress resource.
 */
export function createIngress(context: RenderContext): object {
  const { Values } = context;
  const name = fullname(context);
  const ing = Values.ingress;

  return {
    apiVersion: 'networking.k8s.io/v1',
    kind: 'Ingress',
    metadata: {
      name,
      labels: labels(context),
      ...(ing?.annotations ? { annotations: ing.annotations } : {}),
    },
    spec: {
      ...(ing?.className ? { ingressClassName: ing.className } : {}),
      ...(ing?.tls?.length ? { tls: ing.tls } : {}),
      rules: (ing?.hosts || []).map((host: any) => ({
        host: host.host,
        http: {
          paths: (host.paths || []).map((path: any) => ({
            path: path.path,
            pathType: path.pathType ?? 'ImplementationSpecific',
            backend: {
              service: {
                name,
                port: { number: Values.service?.port ?? 80 },
              },
            },
          })),
        },
      })),
    },
  };
}

/**
 * Create a HorizontalPodAutoscaler resource.
 */
export function createHPA(context: RenderContext): object {
  const { Values } = context;
  const as = Values.autoscaling;
  const name = fullname(context);

  const metrics: object[] = [];

  if (as?.targetCPUUtilizationPercentage) {
    metrics.push({
      type: 'Resource',
      resource: {
        name: 'cpu',
        target: {
          type: 'Utilization',
          averageUtilization: as.targetCPUUtilizationPercentage,
        },
      },
    });
  }

  if (as?.targetMemoryUtilizationPercentage) {
    metrics.push({
      type: 'Resource',
      resource: {
        name: 'memory',
        target: {
          type: 'Utilization',
          averageUtilization: as.targetMemoryUtilizationPercentage,
        },
      },
    });
  }

  return {
    apiVersion: 'autoscaling/v2',
    kind: 'HorizontalPodAutoscaler',
    metadata: {
      name,
      labels: labels(context),
    },
    spec: {
      scaleTargetRef: {
        apiVersion: 'apps/v1',
        kind: 'Deployment',
        name,
      },
      minReplicas: as?.minReplicas ?? 1,
      maxReplicas: as?.maxReplicas ?? 100,
      ...(metrics.length ? { metrics } : {}),
    },
  };
}
`
}

func generateNelmDTS() string {
	return `/**
 * Nelm TypeScript Chart Type Definitions
 *
 * These types define the context object passed to the render() function
 * and the expected return type.
 */

/**
 * The context object passed to the render() function.
 */
export interface RenderContext {
  /**
   * User-provided values merged with chart defaults.
   */
  Values: Values;

  /**
   * Information about the Helm release.
   */
  Release: Release;

  /**
   * Chart metadata from Chart.yaml.
   */
  Chart: ChartMetadata;

  /**
   * Kubernetes cluster capabilities.
   */
  Capabilities: Capabilities;

  /**
   * All files in the chart (excluding templates).
   * Keys are file paths relative to chart root.
   * Values are file contents as Uint8Array.
   */
  Files: Record<string, Uint8Array>;
}

/**
 * User-provided values merged with chart defaults.
 * This interface can be extended to match your values.yaml schema.
 */
export interface Values {
  /** Number of replicas for the deployment */
  replicaCount?: number;

  /** Container image configuration */
  image?: ImageConfig;

  /** Image pull secrets */
  imagePullSecrets?: Array<{ name: string }>;

  /** Override the chart name */
  nameOverride?: string;

  /** Override the full release name */
  fullnameOverride?: string;

  /** Service account configuration */
  serviceAccount?: ServiceAccountConfig;

  /** Pod annotations */
  podAnnotations?: Record<string, string>;

  /** Pod labels */
  podLabels?: Record<string, string>;

  /** Pod security context */
  podSecurityContext?: object;

  /** Container security context */
  securityContext?: object;

  /** Service configuration */
  service?: ServiceConfig;

  /** Ingress configuration */
  ingress?: IngressConfig;

  /** Resource requests and limits */
  resources?: ResourceConfig;

  /** Liveness probe configuration */
  livenessProbe?: ProbeConfig;

  /** Readiness probe configuration */
  readinessProbe?: ProbeConfig;

  /** Autoscaling configuration */
  autoscaling?: AutoscalingConfig;

  /** Volume definitions */
  volumes?: object[];

  /** Volume mount definitions */
  volumeMounts?: object[];

  /** Node selector */
  nodeSelector?: Record<string, string>;

  /** Tolerations */
  tolerations?: object[];

  /** Affinity rules */
  affinity?: object;

  /** Allow any additional values */
  [key: string]: any;
}

export interface ImageConfig {
  repository?: string;
  pullPolicy?: 'Always' | 'IfNotPresent' | 'Never';
  tag?: string;
}

export interface ServiceAccountConfig {
  create?: boolean;
  automount?: boolean;
  annotations?: Record<string, string>;
  name?: string;
}

export interface ServiceConfig {
  type?: 'ClusterIP' | 'NodePort' | 'LoadBalancer' | 'ExternalName';
  port?: number;
}

export interface IngressConfig {
  enabled?: boolean;
  className?: string;
  annotations?: Record<string, string>;
  hosts?: IngressHost[];
  tls?: IngressTLS[];
}

export interface IngressHost {
  host: string;
  paths: IngressPath[];
}

export interface IngressPath {
  path: string;
  pathType?: 'Prefix' | 'Exact' | 'ImplementationSpecific';
}

export interface IngressTLS {
  secretName: string;
  hosts: string[];
}

export interface ResourceConfig {
  limits?: {
    cpu?: string;
    memory?: string;
  };
  requests?: {
    cpu?: string;
    memory?: string;
  };
}

export interface ProbeConfig {
  httpGet?: {
    path: string;
    port: string | number;
  };
  tcpSocket?: {
    port: string | number;
  };
  exec?: {
    command: string[];
  };
  initialDelaySeconds?: number;
  periodSeconds?: number;
  timeoutSeconds?: number;
  successThreshold?: number;
  failureThreshold?: number;
}

export interface AutoscalingConfig {
  enabled?: boolean;
  minReplicas?: number;
  maxReplicas?: number;
  targetCPUUtilizationPercentage?: number;
  targetMemoryUtilizationPercentage?: number;
}

/**
 * Information about the Helm release.
 */
export interface Release {
  /** The release name */
  Name: string;

  /** The release namespace */
  Namespace: string;

  /** The release revision number */
  Revision: number;

  /** True if this is a fresh install */
  IsInstall: boolean;

  /** True if this is an upgrade */
  IsUpgrade: boolean;

  /** The name of the release service (e.g., "Helm") */
  Service: string;
}

/**
 * Chart metadata from Chart.yaml.
 */
export interface ChartMetadata {
  /** The chart name */
  Name: string;

  /** The chart version */
  Version: string;

  /** The application version */
  AppVersion: string;

  /** Chart description */
  Description?: string;

  /** Chart type: "application" or "library" */
  Type?: string;

  /** Chart API version */
  APIVersion?: string;

  /** Chart keywords */
  Keywords?: string[];

  /** Chart home URL */
  Home?: string;

  /** Chart sources */
  Sources?: string[];

  /** Chart maintainers */
  Maintainers?: Maintainer[];

  /** Chart icon URL */
  Icon?: string;

  /** Deprecated flag */
  Deprecated?: boolean;

  /** Chart annotations */
  Annotations?: Record<string, string>;

  /** Kubernetes version constraint */
  KubeVersion?: string;

  /** Chart dependencies */
  Dependencies?: Dependency[];
}

export interface Maintainer {
  name: string;
  email?: string;
  url?: string;
}

export interface Dependency {
  name: string;
  version: string;
  repository?: string;
  condition?: string;
  tags?: string[];
  enabled?: boolean;
  alias?: string;
}

/**
 * Kubernetes cluster capabilities.
 */
export interface Capabilities {
  /** List of supported API versions */
  APIVersions: string[];

  /** Kubernetes version information */
  KubeVersion: KubeVersion;
}

export interface KubeVersion {
  /** Full version string (e.g., "v1.28.0") */
  Version: string;

  /** Major version number */
  Major: string;

  /** Minor version number */
  Minor: string;
}

/**
 * The expected return type from the render() function.
 */
export interface RenderResult {
  /**
   * Array of Kubernetes manifests to render.
   * Each manifest should be a valid Kubernetes resource object.
   * Return an empty array to skip rendering.
   * Return null to skip TypeScript rendering entirely.
   */
  manifests: object[] | null;
}

/**
 * A generic Kubernetes manifest structure.
 */
export interface Manifest {
  apiVersion: string;
  kind: string;
  metadata: ManifestMetadata;
  spec?: object;
  data?: Record<string, string>;
  stringData?: Record<string, string>;
  [key: string]: any;
}

export interface ManifestMetadata {
  name: string;
  namespace?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  [key: string]: any;
}

/**
 * Helper module functions available via require('nelm:helpers').
 */
export interface NelmHelpers {
  /** Encode a string to base64 */
  b64enc(str: string): string;

  /** Decode a base64 string */
  b64dec(str: string): string;

  /** Encode a byte array to base64 */
  b64encBytes(bytes: Uint8Array): string;

  /** Decode a base64 string to byte array */
  b64decBytes(str: string): Uint8Array;

  /** Calculate SHA256 hash of a string */
  sha256sum(str: string): string;

  /** Calculate SHA256 hash of a byte array */
  sha256sumBytes(bytes: Uint8Array): string;

  /** Convert byte array to string */
  bytesToString(bytes: Uint8Array): string;

  /** Convert string to byte array */
  stringToBytes(str: string): Uint8Array;
}

declare module 'nelm:helpers' {
  const helpers: NelmHelpers;
  export = helpers;
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
    "build": "tsc --noEmit",
    "typecheck": "tsc --noEmit"
  },
  "keywords": [
    "helm",
    "nelm",
    "kubernetes",
    "chart"
  ],
  "license": "Apache-2.0"
}
`, chartName, chartName)
}

func generateTSGitignore() string {
	return `# Build artifacts (generated by nelm)
chart_render_main.js

# Dependencies
node_modules/

# TypeScript build output
dist/
`
}

func generateChartYaml(chartName string) string {
	return fmt.Sprintf(`apiVersion: v2
name: %s
description: A Helm chart for Kubernetes

# A chart can be either an 'application' or a 'library' chart.
#
# Application charts are a collection of templates that can be packaged into versioned archives
# to be deployed.
#
# Library charts provide useful utilities or functions for the chart developer. They're included as
# a dependency of application charts to inject those utilities and functions into the rendering
# pipeline. Library charts do not define any templates and therefore cannot be deployed.
type: application

# This is the chart version. This version number should be incremented each time you make changes
# to the chart and its templates, including the app version.
# Versions are expected to follow Semantic Versioning (https://semver.org/)
version: 0.1.0

# This is the version number of the application being deployed. This version number should be
# incremented each time you make changes to the application. Versions are not expected to
# follow Semantic Versioning. They should reflect the version the application is using.
# It is recommended to use it with quotes.
appVersion: "1.16.0"
`, chartName)
}

func generateValuesYaml(chartName string) string {
	return fmt.Sprintf(`# Default values for %s.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

replicaCount: 1

image:
  repository: nginx
  pullPolicy: IfNotPresent
  # Overrides the image tag whose default is the chart appVersion.
  tag: ""

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

serviceAccount:
  # Specifies whether a service account should be created
  create: true
  # Automatically mount a ServiceAccount's API credentials?
  automount: true
  # Annotations to add to the service account
  annotations: {}
  # The name of the service account to use.
  # If not set and create is true, a name is generated using the fullname template
  name: ""

podAnnotations: {}
podLabels: {}

podSecurityContext: {}
  # fsGroup: 2000

securityContext: {}
  # capabilities:
  #   drop:
  #   - ALL
  # readOnlyRootFilesystem: true
  # runAsNonRoot: true
  # runAsUser: 1000

service:
  type: ClusterIP
  port: 80

ingress:
  enabled: false
  className: ""
  annotations: {}
    # kubernetes.io/ingress.class: nginx
    # kubernetes.io/tls-acme: "true"
  hosts:
    - host: chart-example.local
      paths:
        - path: /
          pathType: ImplementationSpecific
  tls: []
  #  - secretName: chart-example-tls
  #    hosts:
  #      - chart-example.local

resources: {}
  # We usually recommend not to specify default resources and to leave this as a conscious
  # choice for the user. This also increases chances charts run on environments with little
  # resources, such as Minikube. If you do want to specify resources, uncomment the following
  # lines, adjust them as necessary, and remove the curly braces after 'resources:'.
  # limits:
  #   cpu: 100m
  #   memory: 128Mi
  # requests:
  #   cpu: 100m
  #   memory: 128Mi

livenessProbe:
  httpGet:
    path: /
    port: http
readinessProbe:
  httpGet:
    path: /
    port: http

autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 100
  targetCPUUtilizationPercentage: 80
  # targetMemoryUtilizationPercentage: 80

# Additional volumes on the output Deployment definition.
volumes: []
# - name: foo
#   secret:
#     secretName: mysecret
#     optional: false

# Additional volumeMounts on the output Deployment definition.
volumeMounts: []
# - name: foo
#   mountPath: "/etc/foo"
#   readOnly: true

nodeSelector: {}

tolerations: []

affinity: {}
`, chartName)
}

func generateHelmignoreWithTS() string {
	return `# Patterns to ignore when building packages.
# This supports shell glob matching, relative path matching, and
# negation (prefixed with !). Only one pattern per line.
.DS_Store
# Common VCS dirs
.git/
.gitignore
.bzr/
.bzrignore
.hg/
.hgignore
.svn/
# Common backup files
*.swp
*.bak
*.tmp
*.orig
*~
# Various IDEs
.project
.idea/
*.tmproj
.vscode/

# TypeScript chart files
ts/node_modules/
ts/dist/
`
}
