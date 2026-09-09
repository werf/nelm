# TypeScript Charts for Nelm

Alternative to Go templates for generating Kubernetes manifests.

## Documents

- [decisions.md](./decisions.md) — Accepted design decisions
- [api.md](./api.md) — HelmContext API and types
- [sdk.md](./sdk.md) — npm packages and types
- [workflow.md](./workflow.md) — Development and deployment workflow
- [cli.md](./cli.md) — CLI commands
- [data-mechanism.md](./data-mechanism.md) — External data fetching

## Overview

TypeScript charts provide a type-safe, scalable alternative to Go templates while maintaining Helm compatibility.

```typescript
import { HelmContext, Manifest } from '@nelm/types'
import { Deployment } from '@nelm/types/apps/v1'
import { Values } from './generated/values.types'

export default function render(ctx: HelmContext<Values>): Manifest[] {
  var deployment: Deployment = {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: {
      name: ctx.Release.Name,
      namespace: ctx.Release.Namespace,
    },
    spec: {
      replicas: ctx.Values.replicas,
      selector: { matchLabels: { app: ctx.Release.Name } },
      template: {
        metadata: { labels: { app: ctx.Release.Name } },
        spec: {
          containers: [{
            name: 'app',
            image: ctx.Values.image.repository + ':' + ctx.Values.image.tag,
          }],
        },
      },
    },
  }

  return [deployment]
}
```

## Key Principles

1. **Pure functions** — `render(ctx) → Manifest[]`
2. **Deterministic** — no network/fs in render, external data via data mechanism
3. **Type safety** — types from `@nelm/types` + generators
4. **Isolation** — subcharts render independently
5. **ES5 target** — goja compatibility, no async/await

## npm Packages

| Package | Purpose |
|---------|---------|
| `@nelm/types` | HelmContext, Manifest, K8s resources |
| `@nelm/crd-to-ts` | Generate types from CRD |
| `json-schema-to-typescript` | Generate Values types |
