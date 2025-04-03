# Nelm reference

- [Annotations](#annotations)
  - [Annotation `werf.io/weight`](#annotation-werfioweight)
  - [Annotation `werf.io/deploy-dependency-<id>`](#annotation-werfiodeploy-dependency-id)
  - [Annotation `<id>.external-dependency.werf.io/resource`](#annotation-idexternal-dependencywerfioresource)
  - [Annotation `<id>.external-dependency.werf.io/name`](#annotation-idexternal-dependencywerfioname)
  - [Annotation `werf.io/sensitive`](#annotation-werfiosensitive)
  - [Annotation `werf.io/track-termination-mode`](#annotation-werfiotrack-termination-mode)
  - [Annotation `werf.io/fail-mode`](#annotation-werfiofail-mode)
  - [Annotation `werf.io/failures-allowed-per-replica`](#annotation-werfiofailures-allowed-per-replica)
  - [Annotation `werf.io/no-activity-timeout`](#annotation-werfiono-activity-timeout)
  - [Annotation `werf.io/log-regex`](#annotation-werfiolog-regex)
  - [Annotation `werf.io/log-regex-for-<container_name>`](#annotation-werfiolog-regex-for-container_name)
  - [Annotation `werf.io/skip-logs`](#annotation-werfioskip-logs)
  - [Annotation `werf.io/skip-logs-for-containers`](#annotation-werfioskip-logs-for-containers)
  - [Annotation `werf.io/show-logs-only-for-containers`](#annotation-werfioshow-logs-only-for-containers)
  - [Annotation `werf.io/show-service-messages`](#annotation-werfioshow-service-messages)
- [Functions](#functions)
  - [Function `werf_secret_file`](#function-werf_secret_file)

## Annotations

### Annotation `werf.io/weight`

Format: `<any number>` \
Default: `0` \
Example: `werf.io/weight: "10"`, `werf.io/weight: "-10"`

Works the same as `helm.sh/hook-weight`, but can be used for both hooks and non-hook resources. Resources with the same weight are grouped together, then the groups deployed one after the other, from low to high weight. Resources in the same group are deployed in parallel. Has higher priority than `helm.sh/hook-weight`, but lower than `werf.io/deploy-dependency-<id>`.

#### Annotation `werf.io/deploy-dependency-<id>`

Format: `state=ready|present[,name=<name>][,namespace=<namespace>][,kind=<kind>][,group=<group>][,version=<version>]` \
Example: \
`werf.io/deploy-dependency-db: state=ready,kind=StatefulSet,name=postgres`, \
`werf.io/deploy-dependency-app: state=present,kind=Deployment,group=apps,version=v1,name=app,namespace=app`

The resource will deploy only after all of its dependencies are satisfied. Waits until the specified resource is just `present` or is also `ready`. More powerful alternative to hooks and `werf.io/weight`. Can only point to resources in the release. Has higher priority than `werf.io/weight` and `helm.sh/hook-weight`.

#### Annotation `<id>.external-dependency.werf.io/resource`

Format: `<kind>[.<version>.<group>]/<name>` \
Example: \
`secret.external-dependency.werf.io/resource: secret/config` \
`someapp.external-dependency.werf.io/resource: deployments.v1.apps/app`

The resource will deploy only after all of its external dependencies are satisfied. Waits until the specified resource is present and ready. Can only point to resources outside the release.

#### Annotation `<id>.external-dependency.werf.io/name`

Format: `<name>` \
Example: `someapp.external-dependency.werf.io/name: someapp-production`

Set the namespace of the external dependency defined by `<id>.external-dependency.werf.io/resource`. `<id>` must match on both annotations. If not specified, the release namespace is used.

#### Annotation `werf.io/sensitive`

Format: `true|false` \
Default: `false`, but for `v1/Secret` — `true` \
Example: `werf.io/sensitive: "true"`

Don't show diffs for the resource.

#### Annotation `werf.io/track-termination-mode`

Format: `WaitUntilResourceReady|NonBlocking` \
Default: `WaitUntilResourceReady` \
Example: `werf.io/track-termination-mode: NonBlocking`

Configure when to stop resource readiness tracking:
* `WaitUntilResourceReady`: wait until the resource is ready.
* `NonBlocking`: don't wait until the resource is ready.

#### Annotation `werf.io/fail-mode`

Format: `FailWholeDeployProcessImmediately|IgnoreAndContinueDeployProcess` \
Default: `FailWholeDeployProcessImmediately` \
Example: `werf.io/fail-mode: IgnoreAndContinueDeployProcess`

Configure what should happen when errors during tracking for the resource exceeded `werf.io/failures-allowed-per-replica`:
* `FailWholeDeployProcessImmediately`: fail the release.
* `IgnoreAndContinueDeployProcess`: do nothing.

#### Annotation `werf.io/failures-allowed-per-replica`

Format: `<any positive number or zero>` \
Default: `1` \
Example: `werf.io/failures-allowed-per-replica: "0"`

Set the number of allowed errors during resource tracking. When exceeded, act according to `werf.io/fail-mode`.

#### Annotation `werf.io/no-activity-timeout`

Format: `<golang duration>` [(reference)](https://pkg.go.dev/time#ParseDuration) \
Default: `4m` \
Example: `werf.io/no-activity-timeout: 8m30s`

Take it as a resource tracking error if no new events or resource updates are received during resource tracking for the specified time.

#### Annotation `werf.io/log-regex`

Format: `<re2 regex>` [(reference)](https://github.com/google/re2/wiki/Syntax) \
Example: `werf.io/log-regex: ".*ERR|err|WARN|warn.*"`

Only show log lines that match the specified regex.

#### Annotation `werf.io/log-regex-for-<container_name>`

Format: `<re2 regex>` [(reference)](https://github.com/google/re2/wiki/Syntax) \
Example: `werf.io/log-regex-for-backend: ".*ERR|err|WARN|warn.*"`

For the specified container only show log lines that match the specified regex.

#### Annotation `werf.io/skip-logs`

Format: `true|false` \
Default: `false` \
Example: `werf.io/skip-logs: "true"`

Don't print container logs during resource tracking.

#### Annotation `werf.io/skip-logs-for-containers`

Format: `<container_name>[,<container_name>...]` \
Example: `werf.io/skip-logs-for-containers: "backend,frontend"`

Don't print logs for specified containers during resource tracking.

#### Annotation `werf.io/show-logs-only-for-containers`

Format: `<container_name>[,<container_name>...]` \
Example: `werf.io/show-logs-only-for-containers: "backend,frontend"`

Print logs only for specified containers during resource tracking.

#### Annotation `werf.io/show-service-messages`

Format: `true|false` \
Default: `false` \
Example: `werf.io/show-service-messages: "true"`

Show resource events during resource tracking.

## Functions

### Function `werf_secret_file`

Format: `werf_secret_file "<filename, relative to secret/ dir>"` \
Example: `config: {{ werf_secret_file "config.yaml" | nindent 4 }}`

Read the specified secret file from the `secret/` directory of the Helm chart.
