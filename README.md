**Nelm** is a Helm 3 alternative. It is a Kubernetes deployment tool that manages Helm Charts and deploys them to Kubernetes and also is the deployment engine of [werf](https://github.com/werf/werf). It can do (almost) everything that Helm does, but better, and even quite some on top of it. Nelm is based on improved and partially rewritten Helm 3 codebase, to introduce:

* `terraform plan`-like capabilities;
* replacement of 3-Way Merge with Server-Side Apply;
* secrets management;
* advanced resource ordering capabilities;
* improved resource state/error tracking;
* continuous printing of logs, events, resource statuses, and errors during deployment;
* lots of fixes for Helm 3 bugs, e.g. ["no matches for kind Deployment in version apps/v1beta1"](https://github.com/helm/helm/issues/7219);
* ... and more.

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
  - [Encrypted values and encrypted files](#encrypted-values-and-encrypted-files)
- [Documentation](#documentation)
- [Known issues](#known-issues)

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
   
## Helm compatibility

Nelm is built upon the Helm 3 codebase with some parts of Helm 3 reimplemented. It is backward-compatible with Helm Charts and Helm Releases.

Helm Charts can be deployed by Nelm with no changes. All the obscure Helm Chart features, such as `lookup` functions, are supported.

To store release information, we use Helm Releases. You can deploy the same release first with Helm, then Nelm and then with Helm and Nelm again, and it will work just fine.

We have a different CLI layout, flags and environment variables, and commands might have a different output format, but we largely support all the same features as Helm.

Helm plugins support is not planned due to technical difficulties with Helm plugins API. Instead, we intend to implement functionality of the most useful plugins natively, like we did with `nelm release plan install` and `nelm chart secret`.

Generally, the migration from Helm to Nelm should be as simple as changing Helm commands to Nelm commands in your CI, for example:

| Helm command | Nelm command equivalent |
| -------- | ------- |
| `helm upgrade --install --atomic --wait -n ns release ./chart` | `nelm release install --auto-rollback -n ns -r release ./chart` |
| `helm uninstall -n ns release` | `nelm release uninstall -n ns -r release` |
| `helm template ./chart` | `nelm chart render ./chart` |
| `helm dependency build` | `nelm chart dependency download` |

## Key features

### Advanced resource ordering

The resource deployment subsystem of Helm is rewritten from scratch in Nelm. During deployment, Nelm builds a Directed Acyclic Graph of all operations we want to perform in a cluster to do a release, which is then executed. A Directed Acyclic Graph allowed us to implement advanced resource ordering capabilities, such as:
* The annotation `werf.io/weight` is similar to `helm.sh/hook-weight`, except it also works for non-hook resources, and resources with the same weight are deployed in parallel.
* The annotation `werf.io/deploy-dependency-<id>` makes Nelm wait for readiness or just presence of another resource in a release before deploying the annotated resource. This is the most powerful and effective way to order resources in Nelm.
* The annotation `<id>.external-dependency.werf.io/resource` allows to wait for readiness of non-release resources, e.g. resources created by third-party operators.
* Helm ordering capabilities, i.e. Helm Hooks and Helm Hook weights, are supported too.

![ordering](resources/images/graph.png)

### 3-Way Merge replaced by Server-Side Apply

Nelm fully replaced problematic [Helm 3-Way Merge](https://helm.sh/docs/faq/changes_since_helm2/#improved-upgrade-strategy-3-way-strategic-merge-patches) with [Server-Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/).

3-Way Merge (3WM) is a client-side mechanism to make a patch for updating a resource in a cluster. Its issues stem from the fact that it has to assume that all previous release manifests were successfully applied to the cluster, which is not always the case. For example, if some resources weren't updated due to being invalid or if a release was aborted too early, then on the next release incorrect 3WM patches might be produced. This results in a "successful" Helm release with wrong changes silently applied to the cluster, which is a very serious issue.

In later versions, Kubernetes introduced Server-Side Apply (SSA) to update resources by making patches server-side by Kubernetes instead of doing so client-side by Helm. SSA solves issues of 3WM and is widely adopted by other deployment tools, like Flux. Unfortunately, it will take a lot of work to replace 3WM with SSA in Helm. But since in Nelm the deployment subsystem was rewritten from scratch, we went SSA-first from the beginning, thus solving long-standing issues of 3-Way Merge.

### Resource state tracking

Nelm has powerful resource tracking built from the ground up:
* Reliable detection of resources readiness, presence, absence or failures.
* Heuristically determined readiness for Custom Resources by analyzing their status fields. Works for about half of Custom Resources. No false positives.
* Some dependent resources, like Pods of Deployments, are automatically found and individually tracked.
* Table with tracked resources current info (statuses, errors, and more) printed every few seconds during deployment.
* Tracking can be configured per resource with annotations.

![tracking](resources/images/nelm-release-install.gif)

### Printing logs and events during deploy

During deployment, Nelm finds Pods of deployed release resources and periodically prints their container logs. Also, with annotation `werf.io/show-service-messages: "true"` resource events are printed too. Log/event printing can be tuned with annotations.

### Release planning

`nelm release plan install` explains exactly what's going to happen in a cluster on the next release. Shows 100% accurate diffs between current and to-be resource versions, utilizing robust dry-run Server-Side Apply instead of client-side trickery.

![planning](resources/images/nelm-release-plan-install.gif)

### Encrypted values and encrypted files

`nelm chart secret` commands manage encrypted values files such as `secret-values.yaml` or encrypted arbitrary files like `secret/mysecret.txt`. These files are decrypted in-memory during templating and can be used in templates as `.Values.my.secret.value` and `{{ werf_secret_file "mysecret.txt" }}` respectively.

## Documentation

Nelm-specific features are described in:
- [CLI.md](docs/CLI.md) overviewing CLI commands available;
- [USAGE.md](docs/USAGE.md) covering encrypted values files and encrypted arbitrary files;
- [REFERENCE.md](docs/REFERENCE.md) listing annotations and functions you can use in Nelm.

For general documentation, see [Helm docs](https://helm.sh/docs/) and [werf docs](https://werf.io/docs/v2/usage/deploy/overview.html).

## Known issues

- Nelm won't work with Kubernetes versions earlier than v1.14. The `ServerSideApply` feature gate should be enabled (it's enabled by default starting from Kubernetes v1.16). This requirement is caused by leveraging the Server-Side Apply (instead of 3-Way Merge in Helm).
