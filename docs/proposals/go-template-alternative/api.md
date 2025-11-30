# HelmContext API

## Main Interface

```typescript
interface HelmContext<V = unknown> {
  // Data
  Values: V
  Release: Release
  Chart: Chart
  Capabilities: Capabilities
  Files: Files

  // Functions (injected from Go)
  lookup<T = unknown>(apiVersion: string, kind: string, namespace: string, name: string): T | null

  // Serialization
  toYaml(obj: unknown): string
  fromYaml<T>(str: string): T
  toJson(obj: unknown): string
  fromJson<T>(str: string): T

  // Encoding
  b64encode(str: string): string
  b64decode(str: string): string

  // Hashing
  sha256(str: string): string
  sha1(str: string): string
  md5(str: string): string

  // String manipulation (Helm-compatible)
  indent(str: string, spaces: number): string
  nindent(str: string, spaces: number): string
  trim(str: string): string
  trimPrefix(str: string, prefix: string): string
  trimSuffix(str: string, suffix: string): string
  upper(str: string): string
  lower(str: string): string
  title(str: string): string
  quote(str: string): string
  squote(str: string): string

  // ... other Helm helpers
}
```

## Release

```typescript
interface Release {
  Name: string
  Namespace: string
  IsUpgrade: boolean
  IsInstall: boolean
  Revision: number
  Service: string  // "Helm" or "Nelm"
}
```

## Chart

```typescript
interface Chart {
  Name: string
  Version: string
  AppVersion: string
  Description: string
  Keywords: string[]
  Home: string
  Sources: string[]
  Icon: string
  Deprecated: boolean
  Type: string  // "application" or "library"
}
```

## Capabilities

```typescript
interface Capabilities {
  KubeVersion: KubeVersion
  APIVersions: APIVersions
  HelmVersion: HelmVersion
}

interface KubeVersion {
  Major: string
  Minor: string
  GitVersion: string  // e.g., "v1.28.3"

  // Semver comparison helpers
  gte(version: string): boolean
  gt(version: string): boolean
  lte(version: string): boolean
  lt(version: string): boolean
  eq(version: string): boolean
}

interface APIVersions {
  list: string[]
  has(apiVersion: string): boolean
}

interface HelmVersion {
  Version: string
  GitCommit: string
  GoVersion: string
}
```

## Files

```typescript
interface Files {
  get(path: string): string
  getBytes(path: string): Uint8Array
  glob(pattern: string): Map<string, string>  // path -> content
  lines(path: string): string[]
  asConfig(pattern?: string): Record<string, string>
  asSecrets(pattern?: string): Record<string, string>  // base64 encoded
}
```

## Manifest

```typescript
interface Manifest {
  apiVersion: string
  kind: string
  metadata: ObjectMeta
  [key: string]: unknown
}

interface ObjectMeta {
  name: string
  namespace?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  ownerReferences?: OwnerReference[]
  finalizers?: string[]
}
```

## Usage Example

```typescript
import { HelmContext, Manifest } from '@nelm/sdk'
import { Values } from './values.types'
import { when } from '@nelm/sdk'

export default function render(ctx: HelmContext<Values>): Manifest[] {
  const labels = {
    'app.kubernetes.io/name': ctx.Chart.Name,
    'app.kubernetes.io/instance': ctx.Release.Name,
    'app.kubernetes.io/version': ctx.Chart.AppVersion,
  }

  return [
    // Deployment
    {
      apiVersion: 'apps/v1',
      kind: 'Deployment',
      metadata: {
        name: ctx.Release.Name,
        namespace: ctx.Release.Namespace,
        labels,
      },
      spec: {
        replicas: ctx.Values.replicas,
        selector: { matchLabels: labels },
        template: {
          metadata: { labels },
          spec: {
            containers: [{
              name: ctx.Chart.Name,
              image: `${ctx.Values.image.repository}:${ctx.Values.image.tag}`,
            }],
          },
        },
      },
    },

    // Service
    {
      apiVersion: 'v1',
      kind: 'Service',
      metadata: {
        name: ctx.Release.Name,
        namespace: ctx.Release.Namespace,
        labels,
      },
      spec: {
        selector: labels,
        ports: [{ port: 80, targetPort: 8080 }],
      },
    },

    // Conditional Ingress
    ...when(ctx.Values.ingress.enabled, [{
      apiVersion: 'networking.k8s.io/v1',
      kind: 'Ingress',
      metadata: {
        name: ctx.Release.Name,
        namespace: ctx.Release.Namespace,
      },
      spec: {
        rules: [{
          host: ctx.Values.ingress.host,
          http: {
            paths: [{
              path: '/',
              pathType: 'Prefix',
              backend: {
                service: {
                  name: ctx.Release.Name,
                  port: { number: 80 },
                },
              },
            }],
          },
        }],
      },
    }]),

    // Conditional: check if CRD exists
    ...when(ctx.Capabilities.APIVersions.has('monitoring.coreos.com/v1'), [{
      apiVersion: 'monitoring.coreos.com/v1',
      kind: 'ServiceMonitor',
      metadata: {
        name: ctx.Release.Name,
        namespace: ctx.Release.Namespace,
      },
      spec: {
        selector: { matchLabels: labels },
        endpoints: [{ port: 'http' }],
      },
    }]),
  ]
}
```
