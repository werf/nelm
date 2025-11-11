<!-- <p align="center">
  <a href="https://godoc.org/github.com/werf/nelm"><img src="https://godoc.org/github.com/werf/nelm?status.svg" alt="GoDoc"></a>
  <a href="https://qlty.sh/gh/werf/projects/nelm"><img src="https://qlty.sh/gh/werf/projects/nelm/coverage.svg" alt="Code Coverage" /></a>
</p> -->

**Nelm** is a Helm 4 alternative. It is a Kubernetes deployment tool that manages Helm Charts and deploys them to Kubernetes. It is also the deployment engine of [werf](https://github.com/werf/werf). Nelm does everything that Helm does, but better, and even quite some on top of it. Nelm is based on an improved and partially rewritten Helm codebase, to introduce:

* `terraform plan`-like capabilities;
* out-of-the-box secrets management;
* advanced resource ordering capabilities;
* advanced resource lifecycle capabilities;
* improved resource state/error tracking;
* continuous printing of logs, events, resource statuses, and errors during deployment;
* fixed hundreds of Helm bugs, e.g. ["no matches for kind Deployment in version apps/v1beta1"](https://github.com/helm/helm/issues/7219);
* performance and stability improvements and more.

The Nelm goal is to provide a modern alternative to Helm, with long-standing issues fixed and many new major features introduced. Nelm moves fast, but our focus remains on Helm Chart and Release compatibility, to ease the migration from Helm.

Nelm is production-ready: as the werf deployment engine, it was battle-tested across thousands of projects for years.

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Install](#install)
- [Quickstart](#quickstart)
- [CLI overview](#cli-overview)
- [Helm compatibility](#helm-compatibility)
- [Key features](#key-features)
  - [Advanced resource ordering](#advanced-resource-ordering)
  - [Advanced resource lifecycle capabilities](#advanced-resource-lifecycle-capabilities)
  - [Resource state tracking](#resource-state-tracking)
  - [Printing logs and events during deploy](#printing-logs-and-events-during-deploy)
  - [Release planning](#release-planning)
  - [Encrypted values and encrypted files](#encrypted-values-and-encrypted-files)
  - [Improved CRD management](#improved-crd-management)
- [Documentation](#documentation)
  - [Usage](#usage)
    - [Encrypted values files](#encrypted-values-files)
    - [Encrypted arbitrary files](#encrypted-arbitrary-files)
  - [Reference](#reference)
    - [Annotation `werf.io/weight`](#annotation-werfioweight)
    - [Annotation `werf.io/deploy-dependency-<id>`](#annotation-werfiodeploy-dependency-id)
    - [Annotation `<id>.external-dependency.werf.io/resource`](#annotation-idexternal-dependencywerfioresource)
    - [Annotation `<id>.external-dependency.werf.io/name`](#annotation-idexternal-dependencywerfioname)
    - [Annotation `werf.io/ownership`](#annotation-werfioownership)
    - [Annotation `werf.io/deploy-on`](#annotation-werfiodeploy-on)
    - [Annotation `werf.io/delete-policy`](#annotation-werfiodelete-policy)
    - [Annotation `werf.io/track-termination-mode`](#annotation-werfiotrack-termination-mode)
    - [Annotation `werf.io/fail-mode`](#annotation-werfiofail-mode)
    - [Annotation `werf.io/failures-allowed-per-replica`](#annotation-werfiofailures-allowed-per-replica)
    - [Annotation `werf.io/no-activity-timeout`](#annotation-werfiono-activity-timeout)
    - [Annotation `werf.io/sensitive`](#annotation-werfiosensitive)
    - [Annotation `werf.io/sensitive-paths`](#annotation-werfiosensitive-paths)
    - [Annotation `werf.io/log-regex`](#annotation-werfiolog-regex)
    - [Annotation `werf.io/log-regex-for-<container_name>`](#annotation-werfiolog-regex-for-container_name)
    - [Annotation `werf.io/log-regex-skip`](#annotation-werfiolog-regex-skip)
    - [Annotation `werf.io/skip-logs`](#annotation-werfioskip-logs)
    - [Annotation `werf.io/skip-logs-for-containers`](#annotation-werfioskip-logs-for-containers)
    - [Annotation `werf.io/show-logs-only-for-number-of-replicas`](#annotation-werfioshow-logs-only-for-number-of-replicas)
    - [Annotation `werf.io/show-logs-only-for-containers`](#annotation-werfioshow-logs-only-for-containers)
    - [Annotation `werf.io/show-service-messages`](#annotation-werfioshow-service-messages)
    - [Function `werf_secret_file`](#function-werf_secret_file)
    - [Function `dump_debug`](#function-dump_debug)
    - [Function `printf_debug`](#function-printf_debug)
    - [Function `include_debug`](#function-include_debug)
    - [Function `tpl_debug`](#function-tpl_debug)
  - [Feature gates](#feature-gates)
    - [Env variable `NELM_FEAT_PREVIEW_V2`](#env-variable-nelm_feat_preview_v2)
    - [Env variable `NELM_FEAT_REMOTE_CHARTS`](#env-variable-nelm_feat_remote_charts)
    - [Env variable `NELM_FEAT_NATIVE_RELEASE_LIST`](#env-variable-nelm_feat_native_release_list)
    - [Env variable `NELM_FEAT_NATIVE_RELEASE_UNINSTALL`](#env-variable-nelm_feat_native_release_uninstall)
    - [Env variable `NELM_FEAT_PERIODIC_STACK_TRACES`](#env-variable-nelm_feat_periodic_stack_traces)
    - [Env variable `NELM_FEAT_FIELD_SENSITIVE`](#env-variable-nelm_feat_field_sensitive)
  - [More information](#more-information)
- [Limitations](#limitations)
- [Future plans](#future-plans)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Install

Follow instructions on [GitHub Releases](https://github.com/werf/nelm/releases).

## Quickstart

1. Create a directory for a new chart:
    ```bash
    mkdir mychart
    cd mychart
    ```

1. Create `Chart.yaml` with the following content:
    ```yaml
    apiVersion: v2
    name: mychart
    version: 1.0.0
    dependencies:
    - name: cert-manager
      version: 1.13.3
      repository: https://charts.jetstack.io
    ```

1. Generate `Chart.lock`:
    ```bash
    nelm chart dependency download
    ```

1. Create `values.yaml` with the following content:
    ```yaml
    cert-manager:
      installCRDs: true
      startupapicheck:
        enabled: false
    ```

1. Deploy the first release:
    ```bash
    nelm release install -n myproject -r myproject
    ```
    ... resources successfully created in the cluster and their readiness is ensured:
    ```smalltalk
    Starting release "myproject" (namespace: "myproject")
   
    ┌ Logs for Pod/myproject-cert-manager-webhook-76c89cc4c7-d8xn4, container/cert-manager-webhook
    │ W0324 14:49:12.719893       1 client_config.go:618] Neither --kubeconfig nor --master was specified.  Using the inClusterConfig.  This might not work.
    │ I0324 14:49:12.743617       1 webhook.go:128] "cert-manager/webhook: using dynamic certificate generating using CA stored in Secret resource" secret_namespace="myproject" secret_name="myproject-cert-manager-webhook-ca"
    │ I0324 14:49:12.743756       1 server.go:147] "cert-manager: listening for insecure healthz connections" address=":6080"
    │ I0324 14:49:12.744309       1 server.go:213] "cert-manager: listening for secure connections" address=":10250"
    │ I0324 14:49:13.747685       1 dynamic_source.go:255] "cert-manager: Updated cert-manager webhook TLS certificate" DNSNames=["myproject-cert-manager-webhook","myproject-cert-manager-webhook.myproject","myproject-cert-manager-webhook.myproject.svc"]
    └ Logs for Pod/myproject-cert-manager-webhook-76c89cc4c7-d8xn4, container/cert-manager-webhook
    
    ┌ Progress status
    │ RESOURCE (→READY)                                                STATE    INFO
    │ Deployment/myproject-cert-manager-webhook                        WAITING  Ready:0/1
    │  • Pod/myproject-cert-manager-webhook-76c89cc4c7-d8xn4           UNKNOWN  Status:Running
    │ ClusterRole/myproject-cert-manager-cainjector                    READY
    │ ClusterRole/myproject-cert-manager-cluster-view                  READY
    │ Role/myproject-cert-manager:leaderelection                       READY    Namespace:kube-system
    │ Role/myproject-cert-manager-webhook:dynamic-serving              READY
    │ RoleBinding/myproject-cert-manager-cainjector:leaderelection     WAITING  Namespace:kube-system
    │ RoleBinding/myproject-cert-manager:leaderelection                WAITING  Namespace:kube-system
    │ RoleBinding/myproject-cert-manager-webhook:dynamic-serving       WAITING
    │ Service/myproject-cert-manager                                   READY
    │ Service/myproject-cert-manager-webhook                           READY
    │ ServiceAccount/myproject-cert-manager                            READY
    │ ServiceAccount/myproject-cert-manager-cainjector                 READY
    │ ServiceAccount/myproject-cert-manager-webhook                    READY
    │ ValidatingWebhookConfiguration/myproject-cert-manager-webhook    READY
    ...
    └ Progress status
    
    ┌ Completed operations
    │ Create resource: ClusterRole/myproject-cert-manager-cainjector
    │ Create resource: ClusterRole/myproject-cert-manager-cluster-view
    │ Create resource: ClusterRole/myproject-cert-manager-controller-approve:cert-manager-io
    │ Create resource: ClusterRole/myproject-cert-manager-controller-certificates
    │ Create resource: ClusterRole/myproject-cert-manager-controller-certificatesigningrequests
    │ Create resource: ClusterRole/myproject-cert-manager-controller-challenges
    ...
    └ Completed operations
    
    Succeeded release "myproject" (namespace: "myproject")
    ```

1. Plan the second release with an increased number of replicas:
    ```bash
    nelm release plan install -n myproject -r myproject --set cert-manager.replicaCount=2
    ```
    ... only the `spec.replicas` field is going to be updated:
    ```smalltalk
    Planning release install "myproject" (namespace: "myproject")

    ┌ Update Deployment/myproject-cert-manager
    │     namespace: myproject
    │   spec:
    │     progressDeadlineSeconds: 600
    │ -   replicas: 1
    │ +   replicas: 2
    │     revisionHistoryLimit: 10
    │     selector:
    │       matchLabels:
    └ Update Deployment/myproject-cert-manager

    Planned changes summary for release "myproject" (namespace: "myproject"):
    - update: 1 resource(s)
    ```

1. Deploy the second release:
    ```bash
    nelm release install -n myproject -r myproject --set cert-manager.replicaCount=2
    ```
    ... only the Deployment is updated:
    ```smalltalk
    Starting release "myproject" (namespace: "myproject")
   
    ┌ Progress status
    │ RESOURCE (→READY)                  STATE  INFO
    │ Deployment/myproject-cert-manager  READY
    └ Progress status
    
    ┌ Completed operations
    │ Update resource: Deployment/myproject-cert-manager
    └ Completed operations
    
    Succeeded release "myproject" (namespace: "myproject")
    ```
   
## CLI overview

```yaml
Release commands:
  release install                    Deploy a chart to Kubernetes.
  release rollback                   Rollback to a previously deployed release.
  release plan install               Plan a release install to Kubernetes.
  release uninstall                  Uninstall a Helm Release from Kubernetes.
  release list                       List all releases in a namespace.
  release history                    Show release history.
  release get                        Get information about a deployed release.

Chart commands:
  chart lint                         Lint a chart.
  chart render                       Render a chart.
  chart download                     Download a chart from a repository.
  chart upload                       Upload a chart to a repository.
  chart pack                         Pack a chart into an archive to distribute via a repository.

Secret commands:
  chart secret key create            Create a new chart secret key.
  chart secret key rotate            Reencrypt secret files with a new secret key.
  chart secret values-file edit      Interactively edit encrypted values file.
  chart secret values-file encrypt   Encrypt values file and print result to stdout.
  chart secret values-file decrypt   Decrypt values file and print result to stdout.
  chart secret file edit             Interactively edit encrypted file.
  chart secret file encrypt          Encrypt file and print result to stdout.
  chart secret file decrypt          Decrypt file and print result to stdout.

Dependency commands:
  chart dependency download          Download chart dependencies from Chart.lock.
  chart dependency update            Update Chart.lock and download chart dependencies.

Repo commands:
  repo add                           Set up a new chart repository.
  repo remove                        Remove a chart repository.
  repo update                        Update info about available charts for all chart repositories.
  repo login                         Log in to an OCI registry with charts.
  repo logout                        Log out from an OCI registry with charts.

Other commands:
  completion bash                    Generate the autocompletion script for bash
  completion fish                    Generate the autocompletion script for fish
  completion powershell              Generate the autocompletion script for powershell
  completion zsh                     Generate the autocompletion script for zsh
  version                            Show version.
```
   
## Helm compatibility

Nelm is built upon the Helm codebase with some parts of Helm reimplemented. It is backward-compatible with Helm Charts and Helm Releases.

Helm Charts can be deployed by Nelm with no changes. All the obscure Helm Chart features, such as `lookup` functions, are supported.

To store release information, Nelm uses Helm Releases. You can deploy the same release with Helm and Nelm interchangeably, and it will work just fine. No migration needed from/to Helm.

Nelm has a different CLI layout, flags and environment variables, but we largely support all the same features as Helm.

Helm plugins support is not planned due to technical difficulties with the Helm plugins API. Instead, we intend to implement functionality of the most useful plugins natively, like we already did with `nelm release plan install` and `nelm chart secret`.

Generally, the migration from Helm to Nelm should be as simple as changing Helm commands to Nelm commands in your CI, for example:

| Helm command | Nelm command equivalent |
| -------- | ------- |
| `helm upgrade --install --atomic --wait -n myns myrls ./chart` | `nelm release install --auto-rollback -n myns -r myrls ./chart` |
| `helm uninstall -n myns myrls` | `nelm release uninstall -n myns -r myrls` |
| `helm template ./chart` | `nelm chart render ./chart` |
| `helm dependency build` | `nelm chart dependency download` |

## Key features

### Advanced resource ordering

The resource deployment subsystem of Helm is rewritten from scratch in Nelm. During the deployment, Nelm builds the Directed Acyclic Graph (DAG) of all operations we want to perform in the cluster to do the release, then the DAG is executed. The DAG allowed us to implement advanced resource ordering capabilities, such as:
* The `werf.io/weight` annotation: similar to `helm.sh/hook-weight`, but also works for non-hook resources. Resources with the same weight deployed in parallel.
* The `werf.io/deploy-dependency-<id>` annotation: do not deploy the annotated resource until the dependency is present or ready. This is the most powerful and effective way to enforce deployment order in Nelm.
* The `<id>.external-dependency.werf.io/resource` annotation: do not deploy the annotated resource until the dependency is ready. The dependency can be an external, non-release resource, e.g. a resource created by a third-party operator.
* Helm Hooks and their weights are supported, too.

![ordering](resources/images/graph.png)

### Advanced resource lifecycle capabilities

Helm doesn't offer any resource lifecycle capabilities, except `helm.sh/resource-policy: keep` and `helm.sh/hook-delete-policy` for Hooks. On top of these, Nelm offers the following:
* The `werf.io/delete-policy` annotation. Inspired by `helm.sh/hook-delete-policy`, but works for any resource. Set `before-creation` to always recreate the resource, `before-creation-if-immutable` to only recreate if the resource is immutable, `succeeded` or `failed` to delete the resource on success or failure. 
* The `werf.io/ownership` annotation. `anyone` allows to get Hook-like behavior for regular resources: don't delete the resource if it is removed from the Chart or when the whole release is removed, and never check or apply release annotations.
* The `werf.io/deploy-on` annotation. Inspired by `helm.sh/hook`. Render and deploy the resource only on install/upgrade/rollback/uninstall in a pre/main/post stage.

These annotations make Helm Hooks obsolete: regular resources can do all the same things now.

### Resource state tracking

Nelm has powerful resource tracking built from the ground up, much more advanced than what Helm has:
* Reliable detection of resources readiness, presence, absence or failures.
* Standard Kubernetes Resources have their own smart status trackers.
* Popular Custom Resources have hand-crafted rules to detect their statuses.
* For unknown Custom Resources, we heuristically determine their readiness by analyzing their status fields. Works for most Custom Resources. No false positives.
* The table with statuses, errors, and other info about currently tracked resources is printed every few seconds during the deployment.

![tracking](resources/images/nelm-release-install.gif)

### Printing logs and events during deploy

During the deployment, Nelm finds Pods of deploying resources and periodically prints their container logs. With annotation `werf.io/show-service-messages: "true"`, resource events are also printed. Can be configured with CLI flags and annotations.

### Release planning

`nelm release plan install` shows exactly what's going to happen in the cluster on the next release. It shows 100% accurate diffs between current and to-be resource versions, utilizing robust dry-run Server-Side Apply instead of client-side trickery.

![planning](resources/images/nelm-release-plan-install.gif)

### Encrypted values and encrypted files

`nelm chart secret` commands manage encrypted values files such as `secret-values.yaml` or encrypted arbitrary files like `secret/mysecret.txt`. These files are decrypted in-memory during templating and can be used in templates as `.Values.my.secret.value` and `{{ werf_secret_file "mysecret.txt" }}`, respectively.

### Improved CRD management

CRDs from the `crds/` directory of the chart deployed not only on the very first release install, but also on release upgrades. Also, CRDs not only can be created, but can be updated as well.

## Documentation

Nelm-specific features are described below. For general documentation, see [Helm docs](https://helm.sh/docs/) and [werf docs](https://werf.io/docs/v2/usage/deploy/overview.html).

### Usage

#### Encrypted values files

Values files can be encrypted and stored in a Helm chart or a git repo. Such values files are decrypted in-memory during templating.

Create a secret key:
```bash
export NELM_SECRET_KEY="$(nelm chart secret key create)"
```

Create a new secret-values file:
```bash
nelm chart secret values-file edit secret-values.yaml
```
... with the following content:
```yaml
password: verysecurepassword123
```

Reference encrypted value in Helm templates:
```yaml
password: {{ .Values.password }}
```

Render the chart:
```bash
nelm chart render
```
```yaml
password: verysecurepassword123
```

NOTE: `$NELM_SECRET_KEY` must be set for any command that encrypts/decrypts secrets, including `nelm chart render`.

#### Encrypted arbitrary files

Arbitrary files can be encrypted and stored in the `secret/` directory of a Helm chart. Such files are decrypted in-memory during templating.

Create a secret key:
```bash
export NELM_SECRET_KEY="$(nelm chart secret key create)"
```

Create a new secret file:
```bash
nelm chart secret file edit secret/config.yaml
```
... with the following content:
```yaml
user: john-doe
password: verysecurepassword123
```

Reference encrypted secret in Helm templates:
```yaml
config: {{ werf_secret_file "config.yaml" | nindent 4 }}
```

Render the chart:
```bash
nelm chart render
```
```yaml
config:
  user: john-doe
  password: verysecurepassword123
```

### Reference

#### Annotation `werf.io/weight`

Format: `<any number>` \
Default: `0` \
Example: `werf.io/weight: "10"`, `werf.io/weight: "-10"`

This annotation works the same as `helm.sh/hook-weight`, but can be used for both hooks and non-hook resources. Resources with the same weight are grouped together, then the groups deployed one after the other, from low to high weight. Resources in the same group are deployed in parallel. This annotation has higher priority than `helm.sh/hook-weight`, but lower than `werf.io/deploy-dependency-<id>`.

#### Annotation `werf.io/deploy-dependency-<id>`

Format: `state=ready|present[,name=<name>][,namespace=<namespace>][,kind=<kind>][,group=<group>][,version=<version>]` \
Example: \
`werf.io/deploy-dependency-db: state=ready,kind=StatefulSet,name=postgres`, \
`werf.io/deploy-dependency-app: state=present,kind=Deployment,group=apps,version=v1,name=app,namespace=app`

The resource will deploy only after all of its dependencies are satisfied. It waits until the specified resource is just `present` or is also `ready`. It serves as a more powerful alternative to hooks and `werf.io/weight`. You can only point to resources in the release. This annotation has higher priority than `werf.io/weight` and `helm.sh/hook-weight`.

#### Annotation `<id>.external-dependency.werf.io/resource`

Format: `<kind>[.<version>.<group>]/<name>` \
Example: \
`secret.external-dependency.werf.io/resource: secret/config` \
`someapp.external-dependency.werf.io/resource: deployments.v1.apps/app`

The resource will deploy only after all of its external dependencies are satisfied. It waits until the specified resource is `present` and `ready`. You can only point to resources outside the release.

#### Annotation `<id>.external-dependency.werf.io/name`

Format: `<name>` \
Example: `someapp.external-dependency.werf.io/name: someapp-production`

Set the namespace of the external dependency defined by `<id>.external-dependency.werf.io/resource`. `<id>` must match on both annotations. If not specified, the release namespace is used.

#### Annotation `werf.io/ownership`

Format: `anyone|release` \
Default: `release` for general resources, `anyone` for hooks and CRDs from `crds/` directory \
Example: `werf.io/ownership: anyone`

Inspired by Helm hooks. Sets the ownership of the resource. `release` means that the resource is deleted if removed from the chart or when the release is uninstalled, and release annotations of the resource are applied/validated during deploy. `anyone` means the opposite: resource is never deleted on uninstall or when removed from the chart, and release annotations are not applied/validated during deploy.

#### Annotation `werf.io/deploy-on`

Format: `[pre-install][,install][,post-install][,pre-upgrade][,upgrade][,post-upgrade][,pre-rollback][,rollback][,post-rollback][,pre-uninstall][,uninstall][,post-uninstall]` \
Default: `install,upgrade,rollback` for general resources, populated from `helm.sh/hook` for hooks \
Example: `werf.io/deploy-on: pre-install,upgrade`

Inspired by `helm.sh/hook`. Render the resource for deployment only on the specified deploy types and stages. Has precedence over `helm.sh/hook`.

Beware that with `werf.io/ownership: release` if the resource is rendered for install, but, for example, not for upgrade, then it is going to be deployed on install, but then deleted on upgrade, so you might want to consider `werf.io/ownership: anyone`.

#### Annotation `werf.io/delete-policy`

Format: `[before-creation][,before-creation-if-immutable][,succeeded][,failed]` \
Default: nothing for general resources (unless Job, then `before-creation-if-immutable`), mapped from `helm.sh/hook-delete-policy` for hooks \
Example: `werf.io/delete-policy: before-creation,succeeded`

Inspired by `helm.sh/hook-delete-policy`. Controls resource deletions during resource deployment. `before-creation` means always recreate the resource, `before-creation-if-immutable` means recreate the resource only when we got "field is immutable" error during its update, `succeeded` means delete the resource at the end of the current deployment stage if the resource was successfully deployed, `failed` means delete the resource if it's readiness check failed. Has precedence over `helm.sh/hook-delete-policy`.

#### Annotation `werf.io/track-termination-mode`

Format: `WaitUntilResourceReady|NonBlocking` \
Default: `WaitUntilResourceReady` \
Example: `werf.io/track-termination-mode: NonBlocking`

Configure when to stop resource readiness tracking:
* `WaitUntilResourceReady`: wait until the resource is `ready`.
* `NonBlocking`: don't wait until the resource is `ready`.

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

#### Annotation `werf.io/sensitive`

Format: `true|false` \
Default: `false`, but for `v1/Secret` — `true` \
Example: `werf.io/sensitive: "true"`

DEPRECATED. Use `werf.io/sensitive-paths` instead.

Don't show diffs for the resource.

`NELM_FEAT_FIELD_SENSITIVE` feature gate alters behavior of this annotation.

#### Annotation `werf.io/sensitive-paths`

Format: `JSONPath,JSONPath,...` \
Example: `werf.io/sensitive-paths: "$.spec.template.spec.containers[*].env[*].value,$.data.*"`

Don't show diffs for resource fields that match specified JSONPath expressions. Overrides the behavior of `werf.io/sensitive`.

#### Annotation `werf.io/log-regex`

Format: `<re2 regex>` [(reference)](https://github.com/google/re2/wiki/Syntax) \
Example: `werf.io/log-regex: ".*ERR|err|WARN|warn.*"`

Only show log lines that match the specified regex.

#### Annotation `werf.io/log-regex-for-<container_name>`

Format: `<re2 regex>` [(reference)](https://github.com/google/re2/wiki/Syntax) \
Example: `werf.io/log-regex-for-backend: ".*ERR|err|WARN|warn.*"`

For the specified container, only show log lines that match the specified regex.

#### Annotation `werf.io/log-regex-skip`

Format: `<re2 regex>` [(reference)](https://github.com/google/re2/wiki/Syntax) \
Example: `werf.io/log-regex-skip: ".*TRACE|trace|DEBUG|debug.*"`

Don't show log lines that match the specified regex.

#### Annotation `werf.io/skip-logs`

Format: `true|false` \
Default: `false` \
Example: `werf.io/skip-logs: "true"`

Don't print container logs during resource tracking.

#### Annotation `werf.io/skip-logs-for-containers`

Format: `<container_name>[,<container_name>...]` \
Example: `werf.io/skip-logs-for-containers: "backend,frontend"`

Don't print logs for specified containers during resource tracking.

#### Annotation `werf.io/show-logs-only-for-number-of-replicas`

Format: `<any positive number or zero>` \
Default: `1` \
Example: `werf.io/show-logs-only-for-number-of-replicas: "999"`

Print logs only for the specified number of replicas during resource tracking. We print logs only for a single replica by default to avoid excessive log output and to optimize resource usage.

#### Annotation `werf.io/show-logs-only-for-containers`

Format: `<container_name>[,<container_name>...]` \
Example: `werf.io/show-logs-only-for-containers: "backend,frontend"`

Print logs only for specified containers during resource tracking.

#### Annotation `werf.io/show-service-messages`

Format: `true|false` \
Default: `false` \
Example: `werf.io/show-service-messages: "true"`

Show resource events during resource tracking.

#### Function `werf_secret_file`

Format: `werf_secret_file "<filename, relative to secret/ dir>"` \
Example: `config: {{ werf_secret_file "config.yaml" | nindent 4 }}`

Read the specified secret file from the `secret/` directory of the Helm chart.

#### Function `dump_debug`

Format: `dump_debug "<value of any type>"` \
Example: `{{ dump_debug $ }}`

If the log level is `debug`, then pretty-dumps the passed value to the logs. Handles just fine any kind of complex types, including .Values, or event root context. Never prints to the templating output.

#### Function `printf_debug`

Format: `printf_debug "<format string>" <args...>` \
Example: `{{ printf_debug "myval: %s" .Values.myval }}`

If the log level is `debug`, then prints the result to the logs. Never prints to the templating output.

#### Function `include_debug`

Format: `include_debug "<template name>" <context>` \
Example: `{{ include_debug "mytemplate" . }}`

Works exactly like the `include` function, but if the log level is `debug`, then also prints various include-related debug information to the logs. Useful for debugging complex includes/defines.

#### Function `tpl_debug`

Format: `tpl_debug "<template string>" <context>` \
Example: `{{ tpl_debug "{{ .Values.myval }}" . }}`

Works exactly like the `tpl` function, but if the log level is `debug`, then also prints various tpl-related debug information to the logs. Useful for debugging complex tpl templates.

### Feature gates

#### Env variable `NELM_FEAT_PREVIEW_V2`

Example:
```shell
export NELM_FEAT_PREVIEW_V2=true
nelm release list
```

Activates all feature gates that will be enabled by default in v2.

#### Env variable `NELM_FEAT_REMOTE_CHARTS`

Example:
```shell
export NELM_FEAT_REMOTE_CHARTS=true
nelm release install -n myproject -r myproject --chart-version 19.1.1 bitnami/nginx
```

Allows specifying not only local, but also remote charts as a command-line argument to commands such as `nelm release install`. Adds the `--chart-version` option as well.

Will be the default in the next major release.

#### Env variable `NELM_FEAT_NATIVE_RELEASE_LIST`

Example:
```shell
export NELM_FEAT_NATIVE_RELEASE_LIST=true
nelm release list
```

Use native Nelm implementation of the `release list` command instead of `helm list` exposed as `release list`. Implementations differ a bit, but serve the same purpose.

Will be the default in the next major release.

#### Env variable `NELM_FEAT_NATIVE_RELEASE_UNINSTALL`

Example:
```shell
export NELM_FEAT_NATIVE_RELEASE_UNINSTALL=true
nelm release uninstall -n myproject -r myproject
```

Use a new native Nelm implementation of the `release uninstall` command. Not fully backwards compatible with previous implementation.

Will be the default in the next major release.

#### Env variable `NELM_FEAT_PERIODIC_STACK_TRACES`

Example:
```shell
export NELM_FEAT_PERIODIC_STACK_TRACES=true
nelm release install -n myproject -r myproject
```

Every few seconds print stack traces of all goroutines. Useful for debugging purposes.

#### Env variable `NELM_FEAT_FIELD_SENSITIVE`

Example:
```shell
export NELM_FEAT_FIELD_SENSITIVE=true
nelm release plan install -n myproject -r myproject
```

When showing diffs for Secrets or `werf.io/sensitive: "true"` annotated resources, instead of hiding the entire resource diff hide only the actual secret fields: `$.data`, `$.stringData`.

Will be the default in the next major release.

### More information

For more information, see [Helm docs](https://helm.sh/docs/) and [werf docs](https://werf.io/docs/v2/usage/deploy/overview.html).

## Limitations

* Nelm requires Server-Side Apply enabled in Kubernetes. It is enabled by default since Kubernetes 1.16. In Kubernetes 1.14-1.15 it can be enabled, but disabled by default. Kubernetes 1.13 and older doesn't have Server-Side Apply, thus Nelm won't work with it.
* *Helm sometimes uses Values from the previous Helm release to deploy a new release*. This is to make Helm easier to use without a proper CI/CD process. This is dangerous, goes against IaC and this is not what users expect. Nelm will never do this: what you explicitly pass via `--values` and `--set` options will be merged with chart values files, then applied to the cluster, as expected.

## Future plans

* The Nelm operator, which can integrate with ArgoCD/Flux.
* An alternative to Helm templating ([#54](https://github.com/werf/nelm/issues/54)).
* Resource patching support ([#115](https://github.com/werf/nelm/issues/115)).
* Downloading charts directly from Git.
* Migrate the built-in secrets management to Mozilla SOPS ([#62](https://github.com/werf/nelm/issues/62)).
