**Nelm** is a **Helm 3** alternative, providing first-class **Helm Chart** support, yet improving on what **Helm 3** offers. Nelm is used as a deployment engine in [werf](https://github.com/werf/werf/). **Nelm** is based on **Helm 3** — some parts of it improved, but some are rewritten from scratch to introduce:
* `terraform plan`-like capabilities;
* replacement of 3-Way Merge with Server-Side Apply;
* advanced resource ordering capabilities;
* secrets management;
* improved resource state/error tracking;
* continuous printing of logs, events, resource statuses and errors during deploy;
* fixes for many Helm 3 issues, e.g. ["no matches for kind Deployment in version apps/v1beta1"](https://github.com/helm/helm/issues/7219)

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Install](#install)
- [Quickstart](#quickstart)
- [CLI Overview](#cli-overview)
- [Helm compatibility](#helm-compatibility)
- [Key features](#key-features)
  - [Advanced resource ordering](#advanced-resource-ordering)
  - [3-Way Merge replaced by Server-Side Apply](#3-way-merge-replaced-by-server-side-apply)
  - [Resource state tracking](#resource-state-tracking)
  - [Printing logs and events during deploy](#printing-logs-and-events-during-deploy)
  - [Release planning](#release-planning)
  - [Encrypted values or arbitrary files](#encrypted-values-or-arbitrary-files)
- [Usage](#usage)
  - [Encrypted values files](#encrypted-values-files)
  - [Encrypted arbitrary files](#encrypted-arbitrary-files)
- [Reference](#reference)
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
  - [Function `werf_secret_file`](#function-werf_secret_file)

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
    ```text
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
    ```text
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
    ```text
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
   
## CLI Overview

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

Nelm is built upon Helm 3 codebase with some parts of Helm 3 reimplemented. We are backwards compatible with Helm Charts and Helm Releases.

Helm Charts can be deployed by Nelm with no changes. We support all of the obscure Helm Chart features, such as `lookup` functions.

To store release information we use Helm Releases. In theory, you can deploy the same release first with Helm, then with Nelm, and then again with Helm, and it will work (not that you'll need this, but still).

We have a different CLI layout, flags and environment variables, and commands might have a different output format, but we largely support all the same features.

Helm plugins support is not planned due to technical difficulties with Helm plugins API, instead we intend to implement functionality of the most useful plugins natively, like we did with `nelm release plan install` and `nelm chart secret`.

Generally, the migration from Helm to Nelm should be as simple as changing Helm commands to Nelm commands, for example:

| Helm command | Nelm command |
| -------- | ------- |
| helm upgrade --install --atomic --wait -n ns release ./chart | nelm release install --auto-rollback -n ns -r release ./chart |
| helm uninstall -n ns release | nelm release uninstall -n ns -r release |
| helm template ./chart | nelm chart render ./chart |
| helm lint ./chart | nelm chart lint ./chart | 
| helm dependency build | nelm chart dependency download |

## Key features

### Advanced resource ordering

The resource deployment subsystem of Helm is rewritten from scratch in Nelm. During deployment, Nelm builds a Directed Acyclic Graph of all operations we want to perform in a cluster to do a release, which is then executed. A Directed Acyclic Graph allowed us to implement advanced resource ordering capabilities, such as:
* The annotation `werf.io/weight` is similar to `helm.sh/hook-weight`, except it works for non-hook resources too and resources with the same weight are deployed in parallel.
* The annotation `werf.io/deploy-dependency-<id>` makes Nelm wait for readiness or just presence of another resource in a release before deploying the annotated resource. This is the most powerful and effective way to order resources in Nelm.
* The annotation `<id>.external-dependency.werf.io/resource` allows to wait for readiness of non-release resources, e.g. resources created by third-party operators.
* Helm ordering capabilities, i.e. Helm Hooks and Helm Hook weights are supported too.

### 3-Way Merge replaced by Server-Side Apply

Nelm fully replaced problematic [Helm 3-Way Merge](https://helm.sh/docs/faq/changes_since_helm2/#improved-upgrade-strategy-3-way-strategic-merge-patches) with [Server-Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/).

3-Way Merge (3WM) is a client-side mechanism to make a patch for updating a resource in a cluster. Its issues stem from the fact that it has to assume that all previous release manifests were successfully applied to the cluster, which is not always the case. For example, if some resources were not updated due to being invalid or if a release was aborted too early, then on the next release incorrect 3WM patches might be produced. This results in a "successful" Helm release with wrong changes silently applied to the cluster, which is a very serious issue.

In recent versions Kubernetes introduced Server-Side Apply (SSA) to update resources by making patches server-side by Kubernetes instead of doing so client-side by Helm. SSA solves issues of 3WM and is widely adopted by other deployment tools, like Flux. Unfortunately, it will take a lot of work to replace 3WM with SSA in Helm. But since in Nelm the deployment subsystem was rewritten from scratch, we went SSA-first from the beginning, thus solving long-standing issues of 3-Way Merge.

### Resource state tracking

Nelm has powerful resource tracking built from the ground up:
* Reliable detection of resources readiness, presence, absence or failures.
* Euristically determined readiness for Custom Resources by analyzing their status fields. Works for about half of Custom Resources. No false positives.
* Some dependent resources, like Pods of Deployments, are automatically found and individually tracked.
* Table with tracked resources current info (statuses, errors and more) printed every few seconds during deploy.
* Tracking can be configured per resource with annotations.

<!-- add gifs -->

### Printing logs and events during deploy

During deploy Nelm finds Pods of deployed release resources and periodically prints their container logs. Also, with annotation `werf.io/show-service-messages: "true"` resource events are printed too. Log/event printing can be tuned with annotations.

<!-- add gifs -->

### Release planning

`nelm release plan install` explains exactly what's going to happen in a cluster on the next release. Shows 100% accurate diffs between current and to-be resource versions, utilizing robust dry-run Server-Side Apply instead of client-side trickery.

<!-- add gifs -->

### Encrypted values or arbitrary files

`nelm chart secret` commands manage encrypted values files such as `secret-values.yaml` or encrypted arbitrary files like `secret/mysecret.txt`. These files are decrypted in-memory on deploy and can be used in templates as `.Values.my.secret.value` and `{{ werf_secret_file "mysecret.txt" }}` respectively.

## Usage

### Encrypted values files

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

### Encrypted arbitrary files

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

## Reference

### Annotation `werf.io/weight`

Format: `<any number>` \
Default: `0` \
Example: `werf.io/weight: "10"`, `werf.io/weight: "-10"`

Works the same as `helm.sh/hook-weight`, but can be used for both hooks and non-hook resources. Resources with the same weight are grouped together, then the groups deployed one after the other, from low to high weight. Resources in the same group are deployed in parallel. Has higher priority than `helm.sh/hook-weight`, but lower than `werf.io/deploy-dependency-<id>`.

### Annotation `werf.io/deploy-dependency-<id>`

Format: `state=ready|present[,name=<name>][,namespace=<namespace>][,kind=<kind>][,group=<group>][,version=<version>]` \
Example: \
`werf.io/deploy-dependency-db: state=ready,kind=StatefulSet,name=postgres`, \
`werf.io/deploy-dependency-app: state=present,kind=Deployment,group=apps,version=v1,name=app,namespace=app`

The resource will deploy only after all of its dependencies are satisfied. Waits until the specified resource is just `present` or is also `ready`. More powerful alternative to hooks and `werf.io/weight`. Can only point to resources in the release. Has higher priority than `werf.io/weight` and `helm.sh/hook-weight`.

### Annotation `<id>.external-dependency.werf.io/resource`

Format: `<kind>[.<version>.<group>]/<name>` \
Example: \
`secret.external-dependency.werf.io/resource: secret/config` \
`someapp.external-dependency.werf.io/resource: deployments.v1.apps/app`

The resource will deploy only after all of its external dependencies are satisfied. Waits until the specified resource is present and ready. Can only point to resources outside of the release.

### Annotation `<id>.external-dependency.werf.io/name`

Format: `<name>` \
Example: `someapp.external-dependency.werf.io/name: someapp-production`

Set the namespace of the external dependency defined by `<id>.external-dependency.werf.io/resource`. `<id>` must match on both annotations. If not specified, the release namespace is used.

### Annotation `werf.io/sensitive`

Format: `true|false` \
Default: `false`, but for `v1/Secret` — `true` \
Example: `werf.io/sensitive: "true"`

Don't show diffs for the resource.

### Annotation `werf.io/track-termination-mode`

Format: `WaitUntilResourceReady|NonBlocking` \
Default: `WaitUntilResourceReady` \
Example: `werf.io/track-termination-mode: NonBlocking`

Configure when to stop resource readiness tracking:
* `WaitUntilResourceReady`: wait until the resource is ready.
* `NonBlocking`: don't wait until the resource is ready.

### Annotation `werf.io/fail-mode`

Format: `FailWholeDeployProcessImmediately|IgnoreAndContinueDeployProcess` \
Default: `FailWholeDeployProcessImmediately` \
Example: `werf.io/fail-mode: IgnoreAndContinueDeployProcess`

Configure what should happen when errors during tracking for the resource exceeded `werf.io/failures-allowed-per-replica`:
* `FailWholeDeployProcessImmediately`: fail the release.
* `IgnoreAndContinueDeployProcess`: do nothing.

### Annotation `werf.io/failures-allowed-per-replica`

Format: `<any positive number or zero>` \
Default: `1` \
Example: `werf.io/failures-allowed-per-replica: "0"`

Set the number of allowed errors during resource tracking. When exceeded, act according to `werf.io/fail-mode`.

### Annotation `werf.io/no-activity-timeout`

Format: `<golang duration>` [(reference)](https://pkg.go.dev/time#ParseDuration) \
Default: `4m` \
Example: `werf.io/no-activity-timeout: 8m30s`

Take it as a resource tracking error if no new events or resource updates received during resource tracking for the specified time.

### Annotation `werf.io/log-regex`

Format: `<re2 regex>` [(reference)](https://github.com/google/re2/wiki/Syntax) \
Example: `werf.io/log-regex: ".*ERR|err|WARN|warn.*"`

Only show log lines that match the specified regex.

### Annotation `werf.io/log-regex-for-<container_name>`

Format: `<re2 regex>` [(reference)](https://github.com/google/re2/wiki/Syntax) \
Example: `werf.io/log-regex-for-backend: ".*ERR|err|WARN|warn.*"`

For the specified container only show log lines that match the specified regex.

### Annotation `werf.io/skip-logs`

Format: `true|false` \
Default: `false` \
Example: `werf.io/skip-logs: "true"`

Don't print container logs during resource tracking.

### Annotation `werf.io/skip-logs-for-containers`

Format: `<container_name>[,<container_name>...]` \
Example: `werf.io/skip-logs-for-containers: "backend,frontend"`

Don't print logs for specified containers during resource tracking.

### Annotation `werf.io/show-logs-only-for-containers`

Format: `<container_name>[,<container_name>...]` \
Example: `werf.io/show-logs-only-for-containers: "backend,frontend"`

Print logs only for specified containers during resource tracking.

### Annotation `werf.io/show-service-messages`

Format: `true|false` \
Default: `false` \
Example: `werf.io/show-service-messages: "true"`

Show resource events during resource tracking.

### Function `werf_secret_file`

Format: `werf_secret_file "<filename, relative to secret/ dir>"` \
Example: `config: {{ werf_secret_file "config.yaml" | nindent 4 }}`

Read the specified secret file from the `secret/` directory of the Helm chart.

