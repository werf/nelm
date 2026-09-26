# HelmContext API

## Main Interface

```typescript
interface HelmContext<V = unknown, D = DataResults> {
  // Data only, no functions
  Values: V
  Release: Release
  Chart: Chart
  Capabilities: Capabilities
  Files: Files
  Data: D  // Results from data() phase
}
```

**Note:** No helper functions in ctx. Define your own as needed.

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
}

interface APIVersions {
  list: string[]
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
  glob(pattern: string): Record<string, string>  // path -> content
  lines(path: string): string[]
}
```

## Data (from data mechanism)

```typescript
type DataResults = Record<string, DataResult>

type DataResult =
  | KubernetesResource
  | KubernetesList
  | boolean
  | null
```

See [data-mechanism.md](./data-mechanism.md) for details.

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
import { HelmContext, Manifest } from '@nelm/types'
import { Deployment } from '@nelm/types/apps/v1'
import { Service } from '@nelm/types/core/v1'
import { Values } from './generated/values.types'

// User-defined helper
function when<T>(condition: boolean, items: T[]): T[] {
  return condition ? items : []
}

export default function render(ctx: HelmContext<Values>): Manifest[] {
  var labels = {
    'app.kubernetes.io/name': ctx.Chart.Name,
    'app.kubernetes.io/instance': ctx.Release.Name,
    'app.kubernetes.io/version': ctx.Chart.AppVersion,
  }

  var deployment: Deployment = {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: {
      name: ctx.Release.Name,
      namespace: ctx.Release.Namespace,
      labels: labels,
    },
    spec: {
      replicas: ctx.Values.replicas,
      selector: { matchLabels: labels },
      template: {
        metadata: { labels: labels },
        spec: {
          containers: [{
            name: ctx.Chart.Name,
            image: ctx.Values.image.repository + ':' + ctx.Values.image.tag,
          }],
        },
      },
    },
  }

  var service: Service = {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: {
      name: ctx.Release.Name,
      namespace: ctx.Release.Namespace,
      labels: labels,
    },
    spec: {
      selector: labels,
      ports: [{ port: 80, targetPort: 8080 }],
    },
  }

  return [
    deployment,
    service,

    // Conditional based on values
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

    // Conditional based on data mechanism
    ...when(ctx.Data.serviceMonitorCRDExists === true, [{
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
