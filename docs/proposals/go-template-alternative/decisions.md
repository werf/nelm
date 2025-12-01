# Design Decisions

## 1. JS Runtime

**Decision:** Use goja (pure Go) instead of quickjs-go or WASM.

**Rationale:**
- Pure Go, no CGO
- Simpler cross-compilation
- No external dependencies
- No need for Wazero

## 2. Build Target

**Decision:** ES5 target, IIFE format, no async/await. esbuild embedded in Nelm.

```bash
# Nelm runs internally:
esbuild src/index.ts --bundle --target=es5 --format=iife --outfile=vendor/bundle.js
```

**Rationale:**
- Maximum compatibility with goja
- Synchronous execution only
- Predictable, deterministic behavior
- esbuild embedded in Nelm — no need to install separately

## 3. Isolation

**Decision:** No network/fs access in JS context.

**Rationale:**
- Security
- Reproducibility
- Deterministic renders
- External data via data mechanism (separate phase)

## 4. Render API

**Decision:** Return-based, not emit-based.

```typescript
export default function render(ctx: HelmContext<Values>): Manifest[] {
  return [manifest1, manifest2, ...]
}
```

**Rationale:**
- Clear, explicit output
- Easy to see what's being created
- Better readability
- Predictable

## 5. Context Design

**Decision:** ctx contains only data, no helper functions.

```typescript
// ctx contains only data
ctx.Values
ctx.Release
ctx.Chart
ctx.Capabilities
ctx.Files
ctx.Data  // from data mechanism
```

**Rationale:**
- Minimal API surface
- User defines own helpers
- Flexibility
- Smaller bundle

## 6. No lookup in render

**Decision:** No lookup() function in render phase. Use data mechanism instead.

**Rationale:**
- Deterministic renders
- No network calls during render
- Better testability
- GitOps friendly (can see diff before deploy)
- See [data-mechanism.md](./data-mechanism.md) for external data

## 7. No built-in helpers

**Decision:** No toYaml, b64encode, sha256, when, etc. in ctx or package.

**Rationale:**
- User defines own helpers as needed
- Output is Manifest[] objects, not YAML strings
- Nelm serializes to YAML
- Minimal package

## 8. Subcharts

**Decision:** Full isolation, no cross-chart access.

**Rationale:**
- Pure functions: each chart `Context → Manifest[]`
- Nelm orchestrates, charts don't know about each other
- Any combination of Go templates + TS works
- Predictable, testable

## 9. Types via npm Packages

**Decision:** Types as npm packages, no helper functions.

| Package | Purpose |
|---------|---------|
| `@nelm/types` | HelmContext, Manifest, K8s resources |
| `@nelm/crd-to-ts` | CLI generator for types from CRD |
| `json-schema-to-typescript` | Values types generation |

**Rationale:**
- npm ecosystem — familiar for TS developers
- Package versioning
- K8s types generated from OpenAPI spec
- Single package for all types

## 10. Values Type Generation

**Decision:** Via npm script using json-schema-to-typescript.

```json
{
  "scripts": {
    "generate:values": "json2ts ../values.schema.json -o src/generated/values.types.ts"
  }
}
```

**Rationale:**
- Node.js already required for development
- Proven library
- Developer controls when to regenerate

## 11. Data Mechanism

**Decision:** Optional `data()` export for external data, executed before render.

```typescript
export function data(ctx: DataContext): DataRequest[] {
  return [{ name: 'secret', type: 'kubernetesResource', ... }]
}

export default function render(ctx: HelmContext<Values>): Manifest[] {
  // ctx.Data.secret available here
}
```

**Rationale:**
- Separates data fetching from rendering
- Render stays deterministic
- Explicit data dependencies
- See [data-mechanism.md](./data-mechanism.md)

## 12. K8s Types Generation

**Decision:** Generate K8s types from OpenAPI spec in CI.

**Rationale:**
- Single source of truth (OpenAPI spec)
- Always up-to-date with K8s versions
- Version managed in CI pipeline
