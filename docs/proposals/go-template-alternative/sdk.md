# @nelm/sdk Package

## Overview

Minimal SDK providing TypeScript types and one helper function. All runtime functions are injected by Nelm (Go) into the JS context.

## Package Structure

```
@nelm/sdk/
  package.json
  index.ts
  index.d.ts
  types/
    context.ts    # HelmContext, Release, Chart, etc.
    manifest.ts   # Manifest, ObjectMeta
    capabilities.ts
    files.ts
```

## Source Code

### index.ts

```typescript
// Re-export all types
export * from './types/context'
export * from './types/manifest'
export * from './types/capabilities'
export * from './types/files'

// The only runtime helper
export function when<T>(condition: boolean, items: T[]): T[] {
  return condition ? items : []
}
```

### types/context.ts

```typescript
import { Capabilities } from './capabilities'
import { Files } from './files'

export interface HelmContext<V = unknown> {
  Values: V
  Release: Release
  Chart: Chart
  Capabilities: Capabilities
  Files: Files

  // Functions injected from Go
  lookup<T = unknown>(apiVersion: string, kind: string, namespace: string, name: string): T | null
  toYaml(obj: unknown): string
  fromYaml<T>(str: string): T
  toJson(obj: unknown): string
  fromJson<T>(str: string): T
  b64encode(str: string): string
  b64decode(str: string): string
  sha256(str: string): string
  sha1(str: string): string
  md5(str: string): string
  indent(str: string, spaces: number): string
  nindent(str: string, spaces: number): string
  trim(str: string): string
  trimPrefix(str: string, prefix: string): string
  trimSuffix(str: string, suffix: string): string
  upper(str: string): string
  lower(str: string): string
  title(str: string): string
  quote(str: string): string
  squote(str: string): string
}

export interface Release {
  Name: string
  Namespace: string
  IsUpgrade: boolean
  IsInstall: boolean
  Revision: number
  Service: string
}

export interface Chart {
  Name: string
  Version: string
  AppVersion: string
  Description: string
  Keywords: string[]
  Home: string
  Sources: string[]
  Icon: string
  Deprecated: boolean
  Type: string
}
```

### types/capabilities.ts

```typescript
export interface Capabilities {
  KubeVersion: KubeVersion
  APIVersions: APIVersions
  HelmVersion: HelmVersion
}

export interface KubeVersion {
  Major: string
  Minor: string
  GitVersion: string

  gte(version: string): boolean
  gt(version: string): boolean
  lte(version: string): boolean
  lt(version: string): boolean
  eq(version: string): boolean
}

export interface APIVersions {
  list: string[]
  has(apiVersion: string): boolean
}

export interface HelmVersion {
  Version: string
  GitCommit: string
  GoVersion: string
}
```

### types/files.ts

```typescript
export interface Files {
  get(path: string): string
  getBytes(path: string): Uint8Array
  glob(pattern: string): Map<string, string>
  lines(path: string): string[]
  asConfig(pattern?: string): Record<string, string>
  asSecrets(pattern?: string): Record<string, string>
}
```

### types/manifest.ts

```typescript
export interface Manifest {
  apiVersion: string
  kind: string
  metadata: ObjectMeta
  [key: string]: unknown
}

export interface ObjectMeta {
  name: string
  namespace?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  ownerReferences?: OwnerReference[]
  finalizers?: string[]
}

export interface OwnerReference {
  apiVersion: string
  kind: string
  name: string
  uid: string
  controller?: boolean
  blockOwnerDeletion?: boolean
}
```

## package.json

```json
{
  "name": "@nelm/sdk",
  "version": "1.0.0",
  "description": "TypeScript SDK for Nelm charts",
  "main": "dist/index.js",
  "types": "dist/index.d.ts",
  "files": [
    "dist"
  ],
  "scripts": {
    "build": "tsc",
    "prepublishOnly": "npm run build"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  },
  "keywords": ["nelm", "helm", "kubernetes", "typescript"],
  "license": "Apache-2.0"
}
```

## Distribution

Published to npm as `@nelm/sdk`.

Developers install as devDependency since it's primarily types:

```bash
npm install --save-dev @nelm/sdk
```
