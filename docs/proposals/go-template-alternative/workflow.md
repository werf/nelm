# Development and Deployment Workflow

## Chart Structure

```
mychart/
  Chart.yaml
  values.yaml
  values.schema.json        # Optional, for type generation
  templates/                # Go templates (optional)
  ts/                       # TypeScript source
    package.json
    tsconfig.json
    node_modules/           # .gitignore
    src/
      generated/
        values.types.ts     # From values.schema.json
        *.types.ts          # From CRDs
      index.ts              # Entry point (render + optional data)
    vendor/
      bundle.js             # ES5 bundle for distribution
```

## Development Workflow

### 1. Initialize TypeScript in Chart

```bash
cd mychart
nelm chart ts init .
```

Creates:
```
ts/
  package.json
  tsconfig.json
  src/
    index.ts
```

Output:
```
Created ts/package.json
Created ts/tsconfig.json
Created ts/src/index.ts

Next steps:
  cd ts
  npm install
  npm run generate:values   # if values.schema.json exists
```

### 2. Install Dependencies

```bash
cd ts
npm install
```

### 3. Generate Types

```bash
# Values types from schema
npm run generate:values

# CRD types (if needed)
npm run generate:crd
```

### 4. Develop

```typescript
import { HelmContext, Manifest } from '@nelm/types'
import { Deployment, Service } from '@nelm/types/apps/v1'
import { Values } from './generated/values.types'

export default function render(ctx: HelmContext<Values>): Manifest[] {
  return [
    // ...
  ]
}
```

### 5. Type Check

```bash
npm run typecheck
```

### 6. Test with Nelm

```bash
cd ..
nelm chart render .
```

## Publishing Workflow

### 1. Bundle for Distribution

```bash
nelm chart publish .
```

Nelm runs embedded esbuild:
```bash
# Internally:
esbuild ts/src/index.ts --bundle --target=es5 --format=iife --outfile=ts/vendor/bundle.js
```

**Note:** esbuild is embedded in Nelm CLI. No need to install separately.

### 2. Upload to Registry

Chart uploaded with `ts/vendor/bundle.js`.

`node_modules/` NOT included.

## Deployment Workflow

### 1. Install Chart

```bash
nelm release install myrepo/mychart
```

### 2. Nelm Renders

1. Load `ts/vendor/bundle.js`
2. If `data` export exists:
   - Execute `data(ctx)` in goja
   - Fetch external data (Go)
   - Populate `ctx.Data`
3. Execute `render(ctx)` in goja
4. Serialize Manifest[] to YAML
5. Combine with Go templates (if any)
6. Deploy

**No Node.js required on deployment machine.**

## Generated Files

### package.json

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
}
```

**Note:** esbuild is embedded in Nelm CLI, not needed in devDependencies.

### tsconfig.json

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "declaration": false,
    "outDir": "dist",
    "rootDir": "src"
  },
  "include": ["src/**/*"]
}
```

### src/index.ts (template)

```typescript
import { HelmContext, Manifest } from '@nelm/types'
// import { Values } from './generated/values.types'

type Values = Record<string, unknown>  // Remove after generate:values

export default function render(ctx: HelmContext<Values>): Manifest[] {
  return [
    {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: ctx.Release.Name,
        namespace: ctx.Release.Namespace,
      },
      data: {
        example: 'value',
      },
    },
  ]
}
```

## .gitignore

```gitignore
ts/node_modules/
```

Note: `ts/vendor/bundle.js` IS committed.

## Build Constraints

- **Target:** ES5 (goja compatibility)
- **Format:** IIFE
- **No async/await**
- **No network/fs in JS**
