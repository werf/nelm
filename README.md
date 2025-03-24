**Nelm** is meant to be a **Helm 3** replacement, providing first-class **Helm-chart** support, yet improving on what **Helm 3** offers.

**Nelm** is a standalone tool, but is also used as the deployment engine in [werf](https://github.com/werf/werf/).

**Nelm** is based on **Helm 3** — some parts of it improved, but some are rewritten from scratch (like the deployment subsystem) to introduce:
* `terraform plan`-like capabilities
* Replacement of 3-Way Merge with Server-Side Apply
* Improved resource tracking built from the ground up
* Advanced resource ordering capabilities
* Flexible resource lifecycle management
* Fixes for numerous Helm 3 issues like [this one](https://github.com/helm/helm/issues/7219) and other improvements

## Install

Follow instructions from [the GitHub Releases page](https://github.com/werf/nelm/releases).

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

1. Check what's going to change in the cluster on the first release:
    ```bash
    nelm release plan install -n myproject -r myproject
    ```
    ```text
    Planning release install "myproject" (namespace: "myproject")

    ┌ Create Service/myproject-cert-manager
    │ + apiVersion: v1
    │ + kind: Service
    │ + metadata:
    │ +   annotations: {}
    │ +   labels:
    │ +     app: cert-manager
    │ +     app.kubernetes.io/component: controller
    │ +     app.kubernetes.io/instance: myproject
    │ +     app.kubernetes.io/managed-by: Helm
    │ +     app.kubernetes.io/name: cert-manager
    │ +     app.kubernetes.io/version: v1.13.3
    │ +     helm.sh/chart: cert-manager-v1.13.3
    │ +   name: myproject-cert-manager
    │ +   namespace: myproject
    │ + spec:
    │ +   ports:
    │ +   - name: tcp-prometheus-servicemonitor
    │ +     port: 9402
    │ +     protocol: TCP
    │ +     targetPort: 9402
    │ +   selector:
    │ +     app.kubernetes.io/component: controller
    │ +     app.kubernetes.io/instance: myproject
    │ +     app.kubernetes.io/name: cert-manager
    │ +   type: ClusterIP
    └ Create Service/myproject-cert-manager

    ...

    Planned changes summary for release "myproject" (namespace: "myproject"):
    - create: 47 resource(s)
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
   
1. Plan the second release:
    ```bash
    nelm release plan install -n myproject -r myproject
    ```
    ... no changes planned since we changed nothing:
    ```text
    Planning release install "myproject" (namespace: "myproject")
    No changes planned for release "myproject" (namespace: "myproject")
    ```
   
1. Try to deploy the second release:
    ```bash
    nelm release install -n myproject -r myproject
    ```
    ... as expected, release is skipped and nothing changed in the cluster:
    ```text
    Starting release "myproject" (namespace: "myproject")
    Skipped release "myproject" (namespace: "myproject"): cluster resources already as desired
    ```

1. Plan the third release with an increased number of replicas:
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

1. Deploy the third release:
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
