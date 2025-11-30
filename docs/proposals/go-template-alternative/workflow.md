# Development and Deployment Workflow

## Chart Structure

```
mychart/
  Chart.yaml
  values.yaml
  values.schema.json      # Optional, for type generation
  templates/              # Go templates (optional)
  ts/                     # TypeScript source
    package.json
    tsconfig.json
    node_modules/         # .gitignore
    src/
      index.ts            # Entry point
      values.types.ts     # Generated from schema
      deployment.ts       # Helper modules
      service.ts
    vendor/
      bundle.js           # Bundled for distribution
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
  npm run generate-types   # if values.schema.json exists
```

### 2. Install Dependencies

```bash
cd ts
npm install
```

### 3. Generate Types (if schema exists)

```bash
npm run generate-types
```

Creates `src/values.types.ts` from `../values.schema.json`.

### 4. Develop

Edit `src/index.ts` and other modules. IDE provides full TypeScript support.

```typescript
import { HelmContext, Manifest, when } from '@nelm/sdk'
import { Values } from './values.types'

export default function render(ctx: HelmContext<Values>): Manifest[] {
  return [
    // ...
  ]
}
```

### 5. Test Locally with Node.js

```bash
npm run dev
# Executes with Node.js, outputs manifests to stdout
```

### 6. Test with QuickJS (Nelm)

```bash
nelm chart render .
# Uses embedded QuickJS runtime
```

### 7. Type Check

```bash
npm run typecheck
# tsc --noEmit
```

## Publishing Workflow

### 1. Bundle for Distribution

```bash
nelm chart publish .
```

Nelm internally runs:
```bash
esbuild ts/src/index.ts --bundle --outfile=ts/vendor/bundle.js --format=esm
```

### 2. Upload to Registry

Chart is uploaded with `ts/vendor/bundle.js` included.

`node_modules/` is NOT included (in .gitignore).

## Deployment Workflow

### 1. Install Chart

```bash
nelm release install myrepo/mychart
```

### 2. Nelm Renders

Under the hood:
1. Nelm loads `ts/vendor/bundle.js`
2. Passes context (Values, Release, etc.) to QuickJS
3. Executes `render(ctx)`
4. Receives manifests array
5. Serializes to YAML
6. Combines with Go templates output (if any)
7. Deploys to cluster

**No Node.js required on deployment machine.**

## Generated Files

### package.json

```json
{
  "name": "mychart-ts",
  "private": true,
  "type": "module",
  "scripts": {
    "generate-types": "json2ts ../values.schema.json -o src/values.types.ts",
    "typecheck": "tsc --noEmit",
    "dev": "tsx src/index.ts",
    "build": "esbuild src/index.ts --bundle --outfile=vendor/bundle.js --format=esm"
  },
  "devDependencies": {
    "@nelm/sdk": "^1.0.0",
    "typescript": "^5.0.0",
    "json-schema-to-typescript": "^15.0.0",
    "tsx": "^4.0.0"
  }
}
```

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
import { HelmContext, Manifest, when } from '@nelm/sdk'
// import { Values } from './values.types'  // Uncomment after generate-types

type Values = Record<string, unknown>  // Remove after generate-types

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

## .gitignore Additions

```gitignore
ts/node_modules/
```

Note: `ts/vendor/bundle.js` IS committed for published charts.
