# Design Decisions

## 1. JS Runtime

**Decision:** Use quickjs-go (CGO bindings) instead of WASM.

**Rationale:**
- Simpler architecture
- No need for Wazero
- CGO is acceptable for Nelm

## 2. Isolation

**Decision:** No network/fs access in JS context.

**Rationale:**
- Security
- Reproducibility
- Only Go-injected functions for external access (lookup, Files)

## 3. Render API

**Decision:** Return-based, not emit-based.

```typescript
// Chosen approach
export default function render(ctx: HelmContext<Values>): Manifest[] {
  return [manifest1, manifest2, ...]
}
```

**Rationale:**
- Clear, explicit output
- Easy to see what's being created
- Better readability
- Predictable

**Helper for conditionals:**
```typescript
import { when } from '@nelm/sdk'

return [
  createDeployment(ctx),
  ...when(ctx.Values.ingress.enabled, [
    createIngress(ctx),
  ]),
]
```

## 4. Context Design

**Decision:** Everything in `ctx`, no imports for runtime functions.

```typescript
// All via ctx
ctx.Values
ctx.Release
ctx.Files.get("config.ini")
ctx.lookup("v1", "Secret", "default", "name")
ctx.toYaml(obj)
ctx.b64encode("data")
```

**Rationale:**
- Single source of truth
- Easy to mock in tests
- Clear what comes from outside
- Consistent with Helm (`.Values`, `.Files`, etc.)

## 5. Subcharts

**Decision:** Full isolation, no cross-chart access.

**Rationale:**
- Pure functions: each chart `Context → Manifest[]`
- Nelm orchestrates, charts don't know about each other
- Any combination of Go templates + TS works
- Predictable, testable

## 6. SDK Structure

**Decision:** SDK provides only types + `when()` helper.

**Rationale:**
- All runtime functions injected by Go (toYaml, b64encode, sha256, etc.)
- SDK is essentially devDependency (types only)
- Minimal bundle size

## 7. Values Type Generation

**Decision:** Via npm script using json-schema-to-typescript.

```json
{
  "scripts": {
    "generate-types": "json2ts ../values.schema.json -o src/values.types.ts"
  }
}
```

**Rationale:**
- Node.js already required for development
- No need to embed in Nelm
- Proven library
- Developer controls when to regenerate

## 8. Helm Helpers Source

**Decision:** All Helm-equivalent helpers (toYaml, b64encode, sha256, etc.) provided by Go.

**Rationale:**
- Single serializer (Go YAML lib) for consistency
- Fewer JS dependencies
- Sync functions (no async issues)
