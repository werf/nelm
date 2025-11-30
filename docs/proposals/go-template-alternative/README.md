# TypeScript Charts for Nelm

Alternative to Go templates for generating Kubernetes manifests.

## Documents

- [decisions.md](./decisions.md) — Accepted design decisions
- [api.md](./api.md) — HelmContext API and types
- [sdk.md](./sdk.md) — @nelm/sdk package structure
- [workflow.md](./workflow.md) — Development and deployment workflow
- [cli.md](./cli.md) — CLI commands

## Overview

TypeScript charts provide a type-safe, scalable alternative to Go templates while maintaining Helm compatibility.

```typescript
import { HelmContext, Manifest } from '@nelm/sdk'
import { Values } from './values.types'

export default function render(ctx: HelmContext<Values>): Manifest[] {
  return [
    {
      apiVersion: 'apps/v1',
      kind: 'Deployment',
      metadata: {
        name: ctx.Release.Name,
        namespace: ctx.Release.Namespace,
      },
      spec: {
        replicas: ctx.Values.replicas,
        // ...
      },
    },
  ]
}
```

## Key Principles

1. **Pure functions** — `render(ctx) → Manifest[]`
2. **Explicit context** — everything via `ctx`, no globals
3. **Isolation** — subcharts render independently
4. **Type safety** — Values types generated from schema
5. **No magic** — predictable, testable code
