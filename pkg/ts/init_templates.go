package ts

import "embed"

const (
	helmignoreContent = `# Patterns to ignore when building packages.
# This supports shell glob matching, relative path matching, and
# negation (prefixed with !). Only one pattern per line.

# TypeScript chart files
ts/node_modules/
ts/.dev/
ts/.pnp.*
ts/.yarn/cache/
ts/.yarn/unplugged/
ts/.yarn/__virtual__/
ts/.yarn/install-state.gz
ts/.yarn/build-state.yml
`
	valuesYamlContent = `replicaCount: 1

image:
  repository: nginx
  tag: latest

service:
  enabled: true
  type: ClusterIP
  port: 80
`
)

var (
	//go:embed scaffold/package.json scaffold/tsconfig*.json scaffold/input.example.yaml scaffold/src/*.ts scaffold/src/*.json
	scaffoldFiles embed.FS

	//go:embed scaffold/values_werf.yaml
	scaffoldExtraValues string
)
