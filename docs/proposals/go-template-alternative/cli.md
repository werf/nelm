# CLI Commands

## TypeScript Chart Commands

### nelm chart ts init

Initialize TypeScript support in a chart.

```bash
nelm chart ts init [path]
```

**Arguments:**
- `path` — Chart directory (default: current directory)

**Creates:**
- `ts/package.json`
- `ts/tsconfig.json`
- `ts/src/index.ts`

**Output:**
```
Created ts/package.json
Created ts/tsconfig.json
Created ts/src/index.ts

Next steps:
  cd ts
  npm install
  npm run generate-types   # if values.schema.json exists
```

### nelm chart render

Render chart manifests (Go templates + TypeScript).

```bash
nelm chart render [path] [flags]
```

**Flags:**
- `--values, -f` — Values file
- `--set` — Set values on command line
- `--output, -o` — Output format (yaml, json)

**Behavior:**
1. Renders Go templates (if `templates/` exists)
2. Renders TypeScript (if `ts/` exists)
3. Combines and outputs manifests

### nelm chart publish

Package and publish chart to registry.

```bash
nelm chart publish [path] [flags]
```

**Behavior:**
1. Bundles TypeScript with esbuild → `ts/vendor/bundle.js`
2. Packages chart
3. Uploads to registry

## Existing Commands (unchanged)

These commands work with both Go templates and TypeScript charts:

```bash
nelm release install <chart> [flags]
nelm release upgrade <release> <chart> [flags]
nelm release uninstall <release> [flags]
nelm release list [flags]
```

## Example Session

```bash
# Create new chart
mkdir mychart && cd mychart
nelm chart create .

# Add TypeScript support
nelm chart ts init .

# Install dependencies
cd ts && npm install

# Generate types from schema
npm run generate-types

# Develop...
# Edit src/index.ts

# Test render
cd ..
nelm chart render . --values my-values.yaml

# Publish
nelm chart publish . --repo myrepo
```
