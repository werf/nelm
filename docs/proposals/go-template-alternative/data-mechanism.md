# Data Mechanism Proposal

## Overview

Mechanism for fetching external data BEFORE render phase, keeping render deterministic and isolated.

**Key constraints:**
- All code is synchronous (no async/await)
- Bundle target: ES5 for goja compatibility

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Nelm CLI (Go)                          │
├─────────────────────────────────────────────────────────────┤
│  1. Load bundle.js (ES5)                                    │
│  2. Check if data() export exists                           │
│  3. If exists: execute data(ctx) in goja                    │
│  4. Execute requests (Go, network access)                   │
│  5. Execute render(ctx) with ctx.Data = results             │
└─────────────────────────────────────────────────────────────┘
```

## Usage

### ts/src/index.ts

```typescript
import { DataContext, DataRequest, HelmContext, Manifest } from '@nelm/types'
import { Values } from './generated/values.types'

// Optional export — if not needed, don't export
export function data(ctx: DataContext<Values>): DataRequest[] {
  var requests: DataRequest[] = []

  // Fetch existing secret if specified
  if (ctx.Values.existingSecret.name) {
    requests.push({
      name: 'existingSecret',
      type: 'kubernetesResource',
      apiVersion: 'v1',
      kind: 'Secret',
      namespace: ctx.Values.existingSecret.namespace || ctx.Release.Namespace,
      resourceName: ctx.Values.existingSecret.name,
    })
  }

  // Check if Prometheus CRD exists
  if (ctx.Values.monitoring.enabled) {
    requests.push({
      name: 'prometheusCRDExists',
      type: 'resourceExists',
      apiVersion: 'apiextensions.k8s.io/v1',
      kind: 'CustomResourceDefinition',
      resourceName: 'prometheuses.monitoring.coreos.com',
    })
  }

  return requests
}

// Required export
export default function render(ctx: HelmContext<Values>): Manifest[] {
  var manifests: Manifest[] = []

  // Use collected data
  if (ctx.Data.existingSecret) {
    var secret = ctx.Data.existingSecret as KubernetesResource<'v1', 'Secret'>
    // use secret...
  } else {
    manifests.push(createSecret(ctx))
  }

  if (ctx.Values.monitoring.enabled && ctx.Data.prometheusCRDExists) {
    manifests.push(createServiceMonitor(ctx))
  }

  return manifests
}
```

## Build

```bash
esbuild src/index.ts --bundle --target=es5 --format=iife --outfile=vendor/bundle.js
```

- **target=es5** — goja compatibility
- **format=iife** — single bundle
- **No async/await** — everything synchronous

## Types

### DataContext

Context available during data() phase. Subset of HelmContext.

```typescript
interface DataContext<V = unknown> {
  Values: V
  Release: Release
  Chart: Chart
  Capabilities: Capabilities
  // Note: Files NOT available in data phase
  // Note: Data NOT available (not yet collected)
}
```

### DataRequest

Union type of all supported data requests.

```typescript
type DataRequest =
  | KubernetesResourceRequest
  | KubernetesListRequest
  | ResourceExistsRequest

interface BaseDataRequest {
  /** Unique name to reference in ctx.Data */
  name: string
}
```

### KubernetesResourceRequest

Fetch a single Kubernetes resource.

```typescript
interface KubernetesResourceRequest extends BaseDataRequest {
  type: 'kubernetesResource'
  apiVersion: string
  kind: string
  namespace: string
  resourceName: string
}
```

**Result:** `KubernetesResource | null`

### KubernetesListRequest

Fetch a list of Kubernetes resources.

```typescript
interface KubernetesListRequest extends BaseDataRequest {
  type: 'kubernetesList'
  apiVersion: string
  kind: string
  namespace?: string
  labelSelector?: Record<string, string>
  fieldSelector?: string
  limit?: number
}
```

**Result:** `KubernetesList` (items may be empty array)

### ResourceExistsRequest

Check if a resource or API exists.

```typescript
interface ResourceExistsRequest extends BaseDataRequest {
  type: 'resourceExists'
  apiVersion: string
  kind: string
  namespace?: string
  resourceName?: string
}
```

**Result:** `boolean`

## Result Types

### KubernetesResource

```typescript
interface KubernetesResource<
  ApiVersion extends string = string,
  Kind extends string = string
> {
  apiVersion: ApiVersion
  kind: Kind
  metadata: ObjectMeta
  spec?: unknown
  status?: unknown
  data?: unknown
  [key: string]: unknown
}

interface ObjectMeta {
  name: string
  namespace?: string
  uid: string
  resourceVersion: string
  creationTimestamp: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  ownerReferences?: OwnerReference[]
  finalizers?: string[]
}
```

### KubernetesList

```typescript
interface KubernetesList<
  ApiVersion extends string = string,
  Kind extends string = string
> {
  apiVersion: ApiVersion
  kind: string
  metadata: ListMeta
  items: Array<KubernetesResource<ApiVersion, Kind>>
}

interface ListMeta {
  resourceVersion: string
  continue?: string
  remainingItemCount?: number
}
```

## HelmContext with Data

```typescript
interface HelmContext<V = unknown, D extends DataResults = DataResults> {
  Values: V
  Release: Release
  Chart: Chart
  Capabilities: Capabilities
  Files: Files
  Data: D
}

type DataResults = Record<string, DataResult>

type DataResult =
  | KubernetesResource
  | KubernetesList
  | boolean
  | null
```

## Type-Safe Data Access

Users can define their own Data interface:

```typescript
interface MyChartData {
  existingSecret: KubernetesResource<'v1', 'Secret'> | null
  prometheusCRDExists: boolean
}

export default function render(ctx: HelmContext<Values, MyChartData>): Manifest[] {
  ctx.Data.existingSecret  // typed as Secret | null
  ctx.Data.prometheusCRDExists  // typed as boolean
}
```

## Behavior

### If data() not exported

- Data phase skipped
- `ctx.Data` is empty object `{}`

### If resource not found

| Request type | Result |
|--------------|--------|
| `kubernetesResource` | `null` |
| `kubernetesList` | `{ items: [] }` |
| `resourceExists` | `false` |

### Errors

- Network errors → Nelm fails with error
- RBAC errors → Nelm fails with error
- Invalid request → Nelm fails with error

## Execution Order

```
1. nelm release install mychart
2. Load and merge Values
3. Bundle index.ts with esbuild (target=es5)
4. Load bundle.js in goja
5. If data export exists:
   a. Execute data(ctx)
   b. Validate DataRequest[]
   c. Execute requests against Kubernetes API (Go)
   d. Collect results
6. Execute render(ctx) with ctx.Data populated
7. Serialize Manifest[] to YAML
8. Deploy to cluster
```

## Security

1. **No network in JS** — requests executed by Go
2. **Explicit** — only declared data is fetched
3. **RBAC** — subject to user's Kubernetes permissions
4. **Read-only** — no write operations
5. **Synchronous** — no async operations, predictable execution

## Examples

### Check CRD before creating CR

```typescript
export function data(ctx: DataContext): DataRequest[] {
  return [{
    name: 'serviceMonitorCRD',
    type: 'resourceExists',
    apiVersion: 'apiextensions.k8s.io/v1',
    kind: 'CustomResourceDefinition',
    resourceName: 'servicemonitors.monitoring.coreos.com',
  }]
}

export default function render(ctx: HelmContext<Values>): Manifest[] {
  var manifests = [createDeployment(ctx), createService(ctx)]

  if (ctx.Data.serviceMonitorCRD) {
    manifests.push(createServiceMonitor(ctx))
  }

  return manifests
}
```

### Use existing or create new secret

```typescript
export function data(ctx: DataContext<Values>): DataRequest[] {
  if (!ctx.Values.existingSecretName) return []

  return [{
    name: 'existingSecret',
    type: 'kubernetesResource',
    apiVersion: 'v1',
    kind: 'Secret',
    namespace: ctx.Release.Namespace,
    resourceName: ctx.Values.existingSecretName,
  }]
}

export default function render(ctx: HelmContext<Values>): Manifest[] {
  if (ctx.Data.existingSecret) {
    // Use existing secret name in deployment
  } else {
    // Create new secret
  }
}
```

## Future Extensions (Not in v1)

- `httpRequest` — external HTTP APIs
- `awsSecret` — AWS Secrets Manager
- `vaultSecret` — HashiCorp Vault
- Caching with TTL
- Parallel fetching
