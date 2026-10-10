# Types & Packages

## npm Packages

| Package | Type | Purpose |
|---------|------|---------|
| `@nelm/types` | Types | HelmContext, Manifest, K8s resources |
| `@nelm/crd-to-ts` | CLI generator | Generate types from CRD |
| `json-schema-to-typescript` | CLI generator | Generate Values types from schema |

## @nelm/types

Single package with all types. No helper functions.

### Nelm Types

```typescript
import { HelmContext, Manifest, DataContext, DataRequest } from '@nelm/types'
```

### Kubernetes Types (Generated from OpenAPI)

```typescript
import { Deployment, StatefulSet, DaemonSet } from '@nelm/types/apps/v1'
import { ConfigMap, Secret, Service, Pod } from '@nelm/types/core/v1'
import { Ingress, NetworkPolicy } from '@nelm/types/networking/v1'
import { Job, CronJob } from '@nelm/types/batch/v1'
```

### Package Structure

```
@nelm/types/
  index.ts                # HelmContext, Manifest, DataRequest, etc.
  apps/
    v1.ts                 # Deployment, StatefulSet, DaemonSet, ReplicaSet
  core/
    v1.ts                 # ConfigMap, Secret, Service, Pod, PVC, etc.
  networking/
    v1.ts                 # Ingress, NetworkPolicy, IngressClass
  batch/
    v1.ts                 # Job, CronJob
  rbac.authorization.k8s.io/
    v1.ts                 # Role, ClusterRole, RoleBinding, etc.
  autoscaling/
    v2.ts                 # HorizontalPodAutoscaler
  policy/
    v1.ts                 # PodDisruptionBudget
  ...                     # Generated from K8s OpenAPI spec
```

### Generation

K8s types generated from OpenAPI spec in CI. Version managed in CI pipeline.

## @nelm/crd-to-ts

CLI for generating TypeScript types from Kubernetes CRD.

```bash
# From cluster
npx @nelm/crd-to-ts --crd prometheuses.monitoring.coreos.com -o src/generated/

# From file
npx @nelm/crd-to-ts --file crds/my-crd.yaml -o src/generated/

# From URL
npx @nelm/crd-to-ts --url https://raw.githubusercontent.com/.../crd.yaml -o src/generated/
```

Generates:
```typescript
// src/generated/prometheus.types.ts

export interface Prometheus {
  apiVersion: 'monitoring.coreos.com/v1'
  kind: 'Prometheus'
  metadata: ObjectMeta
  spec: PrometheusSpec
  status?: PrometheusStatus
}

export interface PrometheusSpec {
  replicas?: number
  serviceAccountName?: string
  serviceMonitorSelector?: LabelSelector
  // ... from OpenAPI schema in CRD
}
```

## Project Structure

```
ts/
  src/
    generated/
      values.types.ts       # json-schema-to-typescript
      prometheus.types.ts   # @nelm/crd-to-ts
    index.ts
  package.json
  tsconfig.json
  vendor/
    bundle.js               # ES5 bundle
```

## package.json

```json
{
  "name": "mychart-ts",
  "private": true,
  "scripts": {
    "generate:values": "json2ts ../values.schema.json -o src/generated/values.types.ts",
    "generate:crd": "crd-to-ts --crd servicemonitors.monitoring.coreos.com -o src/generated/",
    "typecheck": "tsc --noEmit",
    "build": "esbuild src/index.ts --bundle --target=es5 --format=iife --outfile=vendor/bundle.js"
  },
  "devDependencies": {
    "@nelm/types": "^1.0.0",
    "@nelm/crd-to-ts": "^1.0.0",
    "typescript": "^5.0.0",
    "json-schema-to-typescript": "^15.0.0"
  }
  // Note: esbuild embedded in Nelm, not needed here
}
```

## Usage Example

```typescript
import { HelmContext, Manifest } from '@nelm/types'
import { Deployment } from '@nelm/types/apps/v1'
import { Service } from '@nelm/types/core/v1'
import { ConfigMap } from '@nelm/types/core/v1'
import { Values } from './generated/values.types'

function when<T>(condition: boolean, items: T[]): T[] {
  return condition ? items : []
}

export default function render(ctx: HelmContext<Values>): Manifest[] {
  var labels = {
    'app.kubernetes.io/name': ctx.Chart.Name,
    'app.kubernetes.io/instance': ctx.Release.Name,
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

  return [
    deployment,
    ...when(ctx.Values.service.enabled, [{
      apiVersion: 'v1',
      kind: 'Service',
      metadata: { name: ctx.Release.Name },
      spec: { selector: labels, ports: [{ port: 80 }] },
    }]),
  ]
}
```
