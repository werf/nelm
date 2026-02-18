<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Commands Overview](#commands-overview)
  - [Release commands](#release-commands)
  - [Chart commands](#chart-commands)
  - [Secret commands](#secret-commands)
  - [Dependency commands](#dependency-commands)
  - [Repo commands](#repo-commands)
  - [Other commands](#other-commands)
- [Commands](#commands)
  - [release install](#release-install)
  - [release rollback](#release-rollback)
  - [release plan install](#release-plan-install)
  - [release uninstall](#release-uninstall)
  - [release list](#release-list)
  - [release history](#release-history)
  - [release get](#release-get)
  - [release plan show](#release-plan-show)
  - [chart lint](#chart-lint)
  - [chart render](#chart-render)
  - [chart download](#chart-download)
  - [chart upload](#chart-upload)
  - [chart pack](#chart-pack)
  - [chart init](#chart-init)
  - [chart secret key create](#chart-secret-key-create)
  - [chart secret key rotate](#chart-secret-key-rotate)
  - [chart secret values-file edit](#chart-secret-values-file-edit)
  - [chart secret values-file encrypt](#chart-secret-values-file-encrypt)
  - [chart secret values-file decrypt](#chart-secret-values-file-decrypt)
  - [chart secret file edit](#chart-secret-file-edit)
  - [chart secret file encrypt](#chart-secret-file-encrypt)
  - [chart secret file decrypt](#chart-secret-file-decrypt)
  - [chart dependency download](#chart-dependency-download)
  - [chart dependency update](#chart-dependency-update)
  - [repo add](#repo-add)
  - [repo remove](#repo-remove)
  - [repo update](#repo-update)
  - [repo login](#repo-login)
  - [repo logout](#repo-logout)
  - [completion bash](#completion-bash)
  - [completion fish](#completion-fish)
  - [completion powershell](#completion-powershell)
  - [completion zsh](#completion-zsh)
  - [version](#version)
- [Feature Gates](#feature-gates)
  - [NELM_FEAT_REMOTE_CHARTS](#nelm_feat_remote_charts)
  - [NELM_FEAT_NATIVE_RELEASE_LIST](#nelm_feat_native_release_list)
  - [NELM_FEAT_PERIODIC_STACK_TRACES](#nelm_feat_periodic_stack_traces)
  - [NELM_FEAT_NATIVE_RELEASE_UNINSTALL](#nelm_feat_native_release_uninstall)
  - [NELM_FEAT_FIELD_SENSITIVE](#nelm_feat_field_sensitive)
  - [NELM_FEAT_PREVIEW_V2](#nelm_feat_preview_v2)
  - [NELM_FEAT_CLEAN_NULL_FIELDS](#nelm_feat_clean_null_fields)
  - [NELM_FEAT_MORE_DETAILED_EXIT_CODE_FOR_PLAN](#nelm_feat_more_detailed_exit_code_for_plan)
  - [NELM_FEAT_RESOURCE_VALIDATION](#nelm_feat_resource_validation)
  - [NELM_FEAT_TYPESCRIPT](#nelm_feat_typescript)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Commands Overview

### Release commands

- [`nelm release install`](#release-install) — Deploy a chart to Kubernetes\.
- [`nelm release rollback`](#release-rollback) — Rollback to a previously deployed release\.
- [`nelm release plan install`](#release-plan-install) — Plan a release install to Kubernetes\.
- [`nelm release uninstall`](#release-uninstall) — Uninstall a Helm Release from Kubernetes\.
- [`nelm release list`](#release-list) — List all releases in a namespace\.
- [`nelm release history`](#release-history) — Show release history\.
- [`nelm release get`](#release-get) — Get information about a deployed release\.
- [`nelm release plan show`](#release-plan-show) — Show plan artifact planned changes\.

### Chart commands

- [`nelm chart lint`](#chart-lint) — Lint a chart\.
- [`nelm chart render`](#chart-render) — Render a chart\.
- [`nelm chart download`](#chart-download) — Download a chart from a repository\.
- [`nelm chart upload`](#chart-upload) — Upload a chart archive to a repository\.
- [`nelm chart pack`](#chart-pack) — Pack a chart into an archive to distribute via a repository\.
- [`nelm chart init`](#chart-init) — Initialize a new chart\.

### Secret commands

- [`nelm chart secret key create`](#chart-secret-key-create) — Create a new chart secret key\.
- [`nelm chart secret key rotate`](#chart-secret-key-rotate) — Reencrypt secret files with a new secret key\.
- [`nelm chart secret values-file edit`](#chart-secret-values-file-edit) — Interactively edit encrypted values file\.
- [`nelm chart secret values-file encrypt`](#chart-secret-values-file-encrypt) — Encrypt values file and print result to stdout\.
- [`nelm chart secret values-file decrypt`](#chart-secret-values-file-decrypt) — Decrypt values file and print result to stdout\.
- [`nelm chart secret file edit`](#chart-secret-file-edit) — Interactively edit encrypted file\.
- [`nelm chart secret file encrypt`](#chart-secret-file-encrypt) — Encrypt file and print result to stdout\.
- [`nelm chart secret file decrypt`](#chart-secret-file-decrypt) — Decrypt file and print result to stdout\.

### Dependency commands

- [`nelm chart dependency download`](#chart-dependency-download) — Download chart dependencies from Chart\.lock\.
- [`nelm chart dependency update`](#chart-dependency-update) — Update Chart\.lock and download chart dependencies\.

### Repo commands

- [`nelm repo add`](#repo-add) — Set up a new chart repository\.
- [`nelm repo remove`](#repo-remove) — Remove a chart repository\.
- [`nelm repo update`](#repo-update) — Update info about available charts for all chart repositories\.
- [`nelm repo login`](#repo-login) — Log in to an OCI registry with charts\.
- [`nelm repo logout`](#repo-logout) — Log out from an OCI registry with charts\.

### Other commands

- [`nelm completion bash`](#completion-bash) — Generate the autocompletion script for bash
- [`nelm completion fish`](#completion-fish) — Generate the autocompletion script for fish
- [`nelm completion powershell`](#completion-powershell) — Generate the autocompletion script for powershell
- [`nelm completion zsh`](#completion-zsh) — Generate the autocompletion script for zsh
- [`nelm version`](#version) — Show version\.

## Commands

### release install

Deploy a chart to Kubernetes\.

**Usage:**

```shell
nelm release install [options...] -n namespace -r release [chart-dir]
```

**Options:**

- `--auto-rollback` (default: `false`)

  Automatically rollback the release on failure\. Var: \$NELM\_RELEASE\_INSTALL\_AUTO\_ROLLBACK

- `--delete-propagation` (default: `"Foreground"`)

  Default delete propagation strategy\. Vars: \$NELM\_DELETE\_PROPAGATION, \$NELM\_RELEASE\_INSTALL\_DELETE\_PROPAGATION

- `--force-adoption` (default: `false`)

  Always adopt resources, even if they belong to a different Helm release\. Vars: \$NELM\_FORCE\_ADOPTION, \$NELM\_RELEASE\_INSTALL\_FORCE\_ADOPTION

- `-n`, `--namespace` (default: `""`)

  The release namespace\. Resources with no namespace will be deployed here\. Vars: \$NELM\_NAMESPACE, \$NELM\_RELEASE\_INSTALL\_NAMESPACE

- `--no-install-crds` (default: `false`)

  Don't install CRDs from "crds/" directories of installed charts\. Var: \$NELM\_RELEASE\_INSTALL\_NO\_INSTALL\_CRDS

- `--no-notes` (default: `false`)

  Don't show release notes at the end of the release\. Var: \$NELM\_RELEASE\_INSTALL\_NO\_NOTES

- `--no-remove-manual-changes` (default: `false`)

  Don't remove fields added manually to the resource in the cluster if fields aren't present in the manifest\. Vars: \$NELM\_NO\_REMOVE\_MANUAL\_CHANGES, \$NELM\_RELEASE\_INSTALL\_NO\_REMOVE\_MANUAL\_CHANGES

- `--plan-lifetime` (default: `2h0m0s`)

  How long plan artifact is valid\. Var: \$NELM\_RELEASE\_INSTALL\_PLAN\_LIFETIME

- `--provenance-keyring` (default: `""`)

  Path to keyring containing public keys to verify chart provenance\. Vars: \$NELM\_PROVENANCE\_KEYRING, \$NELM\_RELEASE\_INSTALL\_PROVENANCE\_KEYRING

- `--provenance-strategy` (default: `"never"`)

  Strategy for provenance verifying\. Vars: \$NELM\_PROVENANCE\_STRATEGY, \$NELM\_RELEASE\_INSTALL\_PROVENANCE\_STRATEGY

- `-r`, `--release` (default: `""`)

  The release name\. Must be unique within the release namespace\. Vars: \$NELM\_RELEASE, \$NELM\_RELEASE\_INSTALL\_RELEASE

- `--release-info-annotations` (default: `{}`)

  Add annotations to release metadata\. Vars: \$NELM\_RELEASE\_INFO\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_INSTALL\_RELEASE\_INFO\_ANNOTATIONS\_\*

- `--release-labels` (default: `{}`)

  Add labels to the release\. What kind of labels depends on the storage driver\. Vars: \$NELM\_RELEASE\_LABELS\_\*, \$NELM\_RELEASE\_INSTALL\_RELEASE\_LABELS\_\*

- `--save-graph-to` (default: `""`)

  Save the Graphviz install graph to a file\. Var: \$NELM\_RELEASE\_INSTALL\_SAVE\_GRAPH\_TO

- `--save-report-to` (default: `""`)

  Save the install report to a file\. Var: \$NELM\_RELEASE\_INSTALL\_SAVE\_REPORT\_TO

- `--save-rollback-graph-to` (default: `""`)

  Save the Graphviz rollback graph to a file\. Var: \$NELM\_RELEASE\_INSTALL\_SAVE\_ROLLBACK\_GRAPH\_TO

- `--show-subchart-notes` (default: `false`)

  Show NOTES\.txt of subcharts after the release\. Var: \$NELM\_RELEASE\_INSTALL\_SHOW\_SUBCHART\_NOTES

- `--templates-allow-dns` (default: `false`)

  Allow performing DNS requests in templating\. Vars: \$NELM\_TEMPLATES\_ALLOW\_DNS, \$NELM\_RELEASE\_INSTALL\_TEMPLATES\_ALLOW\_DNS

- `--timeout` (default: `0s`)

  Fail if not finished in time\. Vars: \$NELM\_TIMEOUT, \$NELM\_RELEASE\_INSTALL\_TIMEOUT

- `--use-plan` (default: `""`)

  Use the gzip\-compressed JSON plan file from the specified path during release install\. Var: \$NELM\_RELEASE\_INSTALL\_USE\_PLAN


**Values options:**

- `--no-default-values` (default: `false`)

  Ignore values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_VALUES, \$NELM\_RELEASE\_INSTALL\_NO\_DEFAULT\_VALUES

- `--set` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. Vars: \$NELM\_SET, \$NELM\_RELEASE\_INSTALL\_SET

- `--set-file` (default: `[]`)

  Set new values, where the key is the value path and the value is the path to the file with the value content\. Vars: \$NELM\_SET\_FILE, \$NELM\_RELEASE\_INSTALL\_SET\_FILE

- `--set-json` (default: `[]`)

  Set new values, where the key is the value path and the value is JSON\. Vars: \$NELM\_SET\_JSON, \$NELM\_RELEASE\_INSTALL\_SET\_JSON

- `--set-literal` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a literal string\. Vars: \$NELM\_SET\_LITERAL, \$NELM\_RELEASE\_INSTALL\_SET\_LITERAL

- `--set-root-json` (default: `[]`)

  Set new keys in the global context \(\$\), where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_ROOT\_JSON, \$NELM\_RELEASE\_INSTALL\_SET\_ROOT\_JSON

- `--set-runtime-json` (default: `[]`)

  Set new keys in \$\.Runtime, where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_RUNTIME\_JSON, \$NELM\_RELEASE\_INSTALL\_SET\_RUNTIME\_JSON

- `--set-string` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a string\. Vars: \$NELM\_SET\_STRING, \$NELM\_RELEASE\_INSTALL\_SET\_STRING

- `--values` (default: `[]`)

  Additional values files\. Vars: \$NELM\_VALUES, \$NELM\_RELEASE\_INSTALL\_VALUES


**Secret options:**

- `--no-decrypt-secrets` (default: `false`)

  Do not decrypt secrets and secret values, pass them as is\. Vars: \$NELM\_NO\_DECRYPT\_SECRETS, \$NELM\_RELEASE\_INSTALL\_NO\_DECRYPT\_SECRETS

- `--no-default-secret-values` (default: `false`)

  Ignore secret\-values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_SECRET\_VALUES, \$NELM\_RELEASE\_INSTALL\_NO\_DEFAULT\_SECRET\_VALUES

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_RELEASE\_INSTALL\_SECRET\_KEY

- `--secret-values` (default: `[]`)

  Secret values files paths\. Vars: \$NELM\_SECRET\_VALUES, \$NELM\_RELEASE\_INSTALL\_SECRET\_VALUES


**Patch options:**

- `--annotations` (default: `{}`)

  Add annotations to all resources\. Vars: \$NELM\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_INSTALL\_ANNOTATIONS\_\*

- `--app-version` (default: `""`)

  Set appVersion of Chart\.yaml\. Vars: \$NELM\_APP\_VERSION, \$NELM\_RELEASE\_INSTALL\_APP\_VERSION

- `--labels` (default: `{}`)

  Add labels to all resources\. Vars: \$NELM\_LABELS\_\*, \$NELM\_RELEASE\_INSTALL\_LABELS\_\*

- `--runtime-annotations` (default: `{}`)

  Add annotations which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_INSTALL\_RUNTIME\_ANNOTATIONS\_\*

- `--runtime-labels` (default: `{}`)

  Add labels which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_LABELS\_\*, \$NELM\_RELEASE\_INSTALL\_RUNTIME\_LABELS\_\*


**Progress options:**

- `--no-final-tracking` (default: `false`)

  By default disable tracking operations that have no create/update/delete resource operations after them, which are most tracking operations, to speed up the release\. Vars: \$NELM\_NO\_FINAL\_TRACKING, \$NELM\_RELEASE\_INSTALL\_NO\_FINAL\_TRACKING

- `--no-pod-logs` (default: `false`)

  Disable Pod logs collection and printing\. Vars: \$NELM\_NO\_POD\_LOGS, \$NELM\_RELEASE\_INSTALL\_NO\_POD\_LOGS

- `--no-show-progress` (default: `false`)

  Don't show logs, events and real\-time info about release resources\. Vars: \$NELM\_NO\_SHOW\_PROGRESS, \$NELM\_RELEASE\_INSTALL\_NO\_SHOW\_PROGRESS

- `--progress-interval` (default: `5s`)

  How often to print new logs, events and real\-time info about release resources\. Vars: \$NELM\_PROGRESS\_INTERVAL, \$NELM\_RELEASE\_INSTALL\_PROGRESS\_INTERVAL

- `--resource-creation-timeout` (default: `0s`)

  Fail if resource creation tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_CREATION\_TIMEOUT, \$NELM\_RELEASE\_INSTALL\_RESOURCE\_CREATION\_TIMEOUT

- `--resource-deletion-timeout` (default: `0s`)

  Fail if resource deletion tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_DELETION\_TIMEOUT, \$NELM\_RELEASE\_INSTALL\_RESOURCE\_DELETION\_TIMEOUT

- `--resource-readiness-timeout` (default: `0s`)

  Fail if resource readiness tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_READINESS\_TIMEOUT, \$NELM\_RELEASE\_INSTALL\_RESOURCE\_READINESS\_TIMEOUT


**Chart repository options:**

- `--chart-repo-basic-password` (default: `""`)

  Basic auth password to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_PASSWORD, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_BASIC\_PASSWORD

- `--chart-repo-basic-username` (default: `""`)

  Basic auth username to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_USERNAME, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_BASIC\_USERNAME

- `--chart-repo-ca` (default: `""`)

  Path to TLS CA file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CA, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_CA

- `--chart-repo-cert` (default: `""`)

  Path to TLS client cert file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CERT, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_CERT

- `--chart-repo-key` (default: `""`)

  Path to TLS client key file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_KEY, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_KEY

- `--chart-repo-pass-creds` (default: `false`)

  Allow sending chart repository credentials to domains different from the chart repository domain when downloading charts\. Vars: \$NELM\_CHART\_REPO\_PASS\_CREDS, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_PASS\_CREDS

- `--chart-repo-request-timeout` (default: `0s`)

  Set timeout for all requests to chart repository\. Vars: \$NELM\_CHART\_REPO\_REQUEST\_TIMEOUT, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_REQUEST\_TIMEOUT

- `--chart-repo-url` (default: `""`)

  Set URL of chart repo to be used to look for chart\. Vars: \$NELM\_CHART\_REPO\_URL, \$NELM\_RELEASE\_INSTALL\_CHART\_REPO\_URL

- `--insecure-chart-repos` (default: `false`)

  Allow insecure HTTP connections to chart repository\. Vars: \$NELM\_INSECURE\_CHART\_REPOS, \$NELM\_RELEASE\_INSTALL\_INSECURE\_CHART\_REPOS

- `--no-update-chart-repos` (default: `false`)

  Don't update chart repositories index\. Vars: \$NELM\_NO\_UPDATE\_CHART\_REPOS, \$NELM\_RELEASE\_INSTALL\_NO\_UPDATE\_CHART\_REPOS

- `--no-verify-chart-repos-tls` (default: `false`)

  Don't verify TLS certificates of chart repository\. Vars: \$NELM\_NO\_VERIFY\_CHART\_REPOS\_TLS, \$NELM\_RELEASE\_INSTALL\_NO\_VERIFY\_CHART\_REPOS\_TLS

- `--oci-chart-repos-creds` (default: `"~/.docker/config.json"`)

  Credentials to access OCI chart repositories\. Vars: \$NELM\_OCI\_CHART\_REPOS\_CREDS, \$NELM\_RELEASE\_INSTALL\_OCI\_CHART\_REPOS\_CREDS


**Kubernetes connection options:**

- `--kube-api-server` (default: `""`)

  Kubernetes API server address\. Vars: \$NELM\_KUBE\_API\_SERVER, \$NELM\_RELEASE\_INSTALL\_KUBE\_API\_SERVER

- `--kube-api-server-tls-name` (default: `""`)

  Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server\. Vars: \$NELM\_KUBE\_API\_SERVER\_TLS\_NAME, \$NELM\_RELEASE\_INSTALL\_KUBE\_API\_SERVER\_TLS\_NAME

- `--kube-auth-password` (default: `""`)

  Basic auth password for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PASSWORD, \$NELM\_RELEASE\_INSTALL\_KUBE\_AUTH\_PASSWORD

- `--kube-auth-provider` (default: `""`)

  Auth provider name for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER, \$NELM\_RELEASE\_INSTALL\_KUBE\_AUTH\_PROVIDER

- `--kube-auth-provider-config` (default: `{}`)

  Auth provider config for authentication in Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER\_CONFIG, \$NELM\_RELEASE\_INSTALL\_KUBE\_AUTH\_PROVIDER\_CONFIG

- `--kube-auth-username` (default: `""`)

  Basic auth username for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_USERNAME, \$NELM\_RELEASE\_INSTALL\_KUBE\_AUTH\_USERNAME

- `--kube-ca` (default: `""`)

  Path to Kubernetes API server TLS CA file\. Vars: \$NELM\_KUBE\_CA, \$NELM\_RELEASE\_INSTALL\_KUBE\_CA

- `--kube-ca-data` (default: `""`)

  Pass Kubernetes API server TLS CA data\. Vars: \$NELM\_KUBE\_CA\_DATA, \$NELM\_RELEASE\_INSTALL\_KUBE\_CA\_DATA

- `--kube-cert` (default: `""`)

  Path to PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT, \$NELM\_RELEASE\_INSTALL\_KUBE\_CERT

- `--kube-cert-data` (default: `""`)

  Pass PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT\_DATA, \$NELM\_RELEASE\_INSTALL\_KUBE\_CERT\_DATA

- `--kube-config` (default: `[]`)

  Kubeconfig path\(s\)\. If multiple specified, their contents are merged\. Vars: \$KUBECONFIG, \$NELM\_KUBE\_CONFIG\_\*, \$NELM\_RELEASE\_INSTALL\_KUBE\_CONFIG\_\*

- `--kube-config-base64` (default: `""`)

  Pass Kubeconfig file content encoded as base64\. Vars: \$NELM\_KUBE\_CONFIG\_BASE\_64, \$NELM\_RELEASE\_INSTALL\_KUBE\_CONFIG\_BASE\_64

- `--kube-context` (default: `""`)

  Use specified Kubeconfig context\. Vars: \$NELM\_KUBE\_CONTEXT, \$NELM\_RELEASE\_INSTALL\_KUBE\_CONTEXT

- `--kube-context-cluster` (default: `""`)

  Use cluster from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_CLUSTER, \$NELM\_RELEASE\_INSTALL\_KUBE\_CONTEXT\_CLUSTER

- `--kube-context-user` (default: `""`)

  Use user from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_USER, \$NELM\_RELEASE\_INSTALL\_KUBE\_CONTEXT\_USER

- `--kube-impersonate-group` (default: `[]`)

  Sets Impersonate\-Group headers when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_GROUP, \$NELM\_RELEASE\_INSTALL\_KUBE\_IMPERSONATE\_GROUP

- `--kube-impersonate-uid` (default: `""`)

  Sets Impersonate\-Uid header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_UID, \$NELM\_RELEASE\_INSTALL\_KUBE\_IMPERSONATE\_UID

- `--kube-impersonate-user` (default: `""`)

  Sets Impersonate\-User header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_USER, \$NELM\_RELEASE\_INSTALL\_KUBE\_IMPERSONATE\_USER

- `--kube-key` (default: `""`)

  Path to PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY, \$NELM\_RELEASE\_INSTALL\_KUBE\_KEY

- `--kube-key-data` (default: `""`)

  Pass PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY\_DATA, \$NELM\_RELEASE\_INSTALL\_KUBE\_KEY\_DATA

- `--kube-proxy-url` (default: `""`)

  Proxy URL to use for proxying all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_PROXY\_URL, \$NELM\_RELEASE\_INSTALL\_KUBE\_PROXY\_URL

- `--kube-request-timeout` (default: `0s`)

  Timeout for all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_REQUEST\_TIMEOUT, \$NELM\_RELEASE\_INSTALL\_KUBE\_REQUEST\_TIMEOUT

- `--kube-token` (default: `""`)

  Bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN, \$NELM\_RELEASE\_INSTALL\_KUBE\_TOKEN

- `--kube-token-path` (default: `""`)

  Path to file with bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN\_PATH, \$NELM\_RELEASE\_INSTALL\_KUBE\_TOKEN\_PATH

- `--no-verify-kube-tls` (default: `false`)

  Don't verify TLS certificates of Kubernetes API\. Vars: \$NELM\_NO\_VERIFY\_KUBE\_TLS, \$NELM\_RELEASE\_INSTALL\_NO\_VERIFY\_KUBE\_TLS


**Performance options:**

- `--kube-burst-limit` (default: `100`)

  Burst limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_BURST\_LIMIT, \$NELM\_RELEASE\_INSTALL\_KUBE\_BURST\_LIMIT

- `--kube-qps-limit` (default: `30`)

  Queries Per Second limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_QPS\_LIMIT, \$NELM\_RELEASE\_INSTALL\_KUBE\_QPS\_LIMIT

- `--network-parallelism` (default: `30`)

  Limit of network\-related tasks to run in parallel\. Vars: \$NELM\_NETWORK\_PARALLELISM, \$NELM\_RELEASE\_INSTALL\_NETWORK\_PARALLELISM


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_RELEASE\_INSTALL\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_RELEASE\_INSTALL\_LOG\_LEVEL

- `--release-history-limit` (default: `10`)

  Limit the number of releases in release history\. When limit is exceeded the oldest releases are deleted\. Release resources are not affected\. Var: \$NELM\_RELEASE\_HISTORY\_LIMIT

- `--release-storage` (default: `""`)

  How releases should be stored\. Var: \$NELM\_RELEASE\_STORAGE

- `--release-storage-sql-connection` (default: `""`)

  SQL connection string for MySQL release storage driver\. Var: \$NELM\_RELEASE\_STORAGE\_SQL\_CONNECTION

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### release rollback

Rollback to a previously deployed release\. Choose the last successful revision \(except the very last revision\), by default\.

**Usage:**

```shell
nelm release rollback [options...] -n namespace -r release [revision]
```

**Options:**

- `--delete-propagation` (default: `"Foreground"`)

  Default delete propagation strategy\. Vars: \$NELM\_DELETE\_PROPAGATION, \$NELM\_RELEASE\_ROLLBACK\_DELETE\_PROPAGATION

- `--force-adoption` (default: `false`)

  Always adopt resources, even if they belong to a different Helm release\. Vars: \$NELM\_FORCE\_ADOPTION, \$NELM\_RELEASE\_ROLLBACK\_FORCE\_ADOPTION

- `-n`, `--namespace` (default: `""`)

  The release namespace\. Resources with no namespace will be deployed here\. Vars: \$NELM\_NAMESPACE, \$NELM\_RELEASE\_ROLLBACK\_NAMESPACE

- `--no-notes` (default: `false`)

  Don't show release notes at the end of the release\. Var: \$NELM\_RELEASE\_ROLLBACK\_NO\_NOTES

- `--no-remove-manual-changes` (default: `false`)

  Don't remove fields added manually to the resource in the cluster if fields aren't present in the manifest\. Vars: \$NELM\_NO\_REMOVE\_MANUAL\_CHANGES, \$NELM\_RELEASE\_ROLLBACK\_NO\_REMOVE\_MANUAL\_CHANGES

- `-r`, `--release` (default: `""`)

  The release name\. Must be unique within the release namespace\. Vars: \$NELM\_RELEASE, \$NELM\_RELEASE\_ROLLBACK\_RELEASE

- `--release-info-annotations` (default: `{}`)

  Add annotations to release metadata\. Vars: \$NELM\_RELEASE\_INFO\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_ROLLBACK\_RELEASE\_INFO\_ANNOTATIONS\_\*

- `--release-labels` (default: `{}`)

  Add labels to the release\. What kind of labels depends on the storage driver\. Vars: \$NELM\_RELEASE\_LABELS\_\*, \$NELM\_RELEASE\_ROLLBACK\_RELEASE\_LABELS\_\*

- `--save-graph-to` (default: `""`)

  Save the Graphviz rollback graph to a file\. Var: \$NELM\_RELEASE\_ROLLBACK\_SAVE\_GRAPH\_TO

- `--save-report-to` (default: `""`)

  Save the rollback report to a file\. Var: \$NELM\_RELEASE\_ROLLBACK\_SAVE\_REPORT\_TO

- `--save-rollback-graph-to` (default: `""`)

  Save the Graphviz rollback graph to a file\. Var: \$NELM\_RELEASE\_ROLLBACK\_SAVE\_ROLLBACK\_GRAPH\_TO

- `--timeout` (default: `0s`)

  Fail if not finished in time\. Vars: \$NELM\_TIMEOUT, \$NELM\_RELEASE\_ROLLBACK\_TIMEOUT


**Patch options:**

- `--runtime-annotations` (default: `{}`)

  Add annotations which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_ROLLBACK\_RUNTIME\_ANNOTATIONS\_\*

- `--runtime-labels` (default: `{}`)

  Add labels which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_LABELS\_\*, \$NELM\_RELEASE\_ROLLBACK\_RUNTIME\_LABELS\_\*


**Progress options:**

- `--no-final-tracking` (default: `false`)

  By default disable tracking operations that have no create/update/delete resource operations after them, which are most tracking operations, to speed up the release\. Vars: \$NELM\_NO\_FINAL\_TRACKING, \$NELM\_RELEASE\_ROLLBACK\_NO\_FINAL\_TRACKING

- `--no-pod-logs` (default: `false`)

  Disable Pod logs collection and printing\. Vars: \$NELM\_NO\_POD\_LOGS, \$NELM\_RELEASE\_ROLLBACK\_NO\_POD\_LOGS

- `--no-show-progress` (default: `false`)

  Don't show logs, events and real\-time info about release resources\. Vars: \$NELM\_NO\_SHOW\_PROGRESS, \$NELM\_RELEASE\_ROLLBACK\_NO\_SHOW\_PROGRESS

- `--progress-interval` (default: `5s`)

  How often to print new logs, events and real\-time info about release resources\. Vars: \$NELM\_PROGRESS\_INTERVAL, \$NELM\_RELEASE\_ROLLBACK\_PROGRESS\_INTERVAL

- `--resource-creation-timeout` (default: `0s`)

  Fail if resource creation tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_CREATION\_TIMEOUT, \$NELM\_RELEASE\_ROLLBACK\_RESOURCE\_CREATION\_TIMEOUT

- `--resource-deletion-timeout` (default: `0s`)

  Fail if resource deletion tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_DELETION\_TIMEOUT, \$NELM\_RELEASE\_ROLLBACK\_RESOURCE\_DELETION\_TIMEOUT

- `--resource-readiness-timeout` (default: `0s`)

  Fail if resource readiness tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_READINESS\_TIMEOUT, \$NELM\_RELEASE\_ROLLBACK\_RESOURCE\_READINESS\_TIMEOUT


**Kubernetes connection options:**

- `--kube-api-server` (default: `""`)

  Kubernetes API server address\. Vars: \$NELM\_KUBE\_API\_SERVER, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_API\_SERVER

- `--kube-api-server-tls-name` (default: `""`)

  Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server\. Vars: \$NELM\_KUBE\_API\_SERVER\_TLS\_NAME, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_API\_SERVER\_TLS\_NAME

- `--kube-auth-password` (default: `""`)

  Basic auth password for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PASSWORD, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_AUTH\_PASSWORD

- `--kube-auth-provider` (default: `""`)

  Auth provider name for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_AUTH\_PROVIDER

- `--kube-auth-provider-config` (default: `{}`)

  Auth provider config for authentication in Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER\_CONFIG, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_AUTH\_PROVIDER\_CONFIG

- `--kube-auth-username` (default: `""`)

  Basic auth username for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_USERNAME, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_AUTH\_USERNAME

- `--kube-ca` (default: `""`)

  Path to Kubernetes API server TLS CA file\. Vars: \$NELM\_KUBE\_CA, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CA

- `--kube-ca-data` (default: `""`)

  Pass Kubernetes API server TLS CA data\. Vars: \$NELM\_KUBE\_CA\_DATA, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CA\_DATA

- `--kube-cert` (default: `""`)

  Path to PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CERT

- `--kube-cert-data` (default: `""`)

  Pass PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT\_DATA, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CERT\_DATA

- `--kube-config` (default: `[]`)

  Kubeconfig path\(s\)\. If multiple specified, their contents are merged\. Vars: \$KUBECONFIG, \$NELM\_KUBE\_CONFIG\_\*, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CONFIG\_\*

- `--kube-config-base64` (default: `""`)

  Pass Kubeconfig file content encoded as base64\. Vars: \$NELM\_KUBE\_CONFIG\_BASE\_64, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CONFIG\_BASE\_64

- `--kube-context` (default: `""`)

  Use specified Kubeconfig context\. Vars: \$NELM\_KUBE\_CONTEXT, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CONTEXT

- `--kube-context-cluster` (default: `""`)

  Use cluster from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_CLUSTER, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CONTEXT\_CLUSTER

- `--kube-context-user` (default: `""`)

  Use user from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_USER, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_CONTEXT\_USER

- `--kube-impersonate-group` (default: `[]`)

  Sets Impersonate\-Group headers when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_GROUP, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_IMPERSONATE\_GROUP

- `--kube-impersonate-uid` (default: `""`)

  Sets Impersonate\-Uid header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_UID, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_IMPERSONATE\_UID

- `--kube-impersonate-user` (default: `""`)

  Sets Impersonate\-User header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_USER, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_IMPERSONATE\_USER

- `--kube-key` (default: `""`)

  Path to PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_KEY

- `--kube-key-data` (default: `""`)

  Pass PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY\_DATA, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_KEY\_DATA

- `--kube-proxy-url` (default: `""`)

  Proxy URL to use for proxying all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_PROXY\_URL, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_PROXY\_URL

- `--kube-request-timeout` (default: `0s`)

  Timeout for all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_REQUEST\_TIMEOUT, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_REQUEST\_TIMEOUT

- `--kube-token` (default: `""`)

  Bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_TOKEN

- `--kube-token-path` (default: `""`)

  Path to file with bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN\_PATH, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_TOKEN\_PATH

- `--no-verify-kube-tls` (default: `false`)

  Don't verify TLS certificates of Kubernetes API\. Vars: \$NELM\_NO\_VERIFY\_KUBE\_TLS, \$NELM\_RELEASE\_ROLLBACK\_NO\_VERIFY\_KUBE\_TLS


**Performance options:**

- `--kube-burst-limit` (default: `100`)

  Burst limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_BURST\_LIMIT, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_BURST\_LIMIT

- `--kube-qps-limit` (default: `30`)

  Queries Per Second limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_QPS\_LIMIT, \$NELM\_RELEASE\_ROLLBACK\_KUBE\_QPS\_LIMIT

- `--network-parallelism` (default: `30`)

  Limit of network\-related tasks to run in parallel\. Vars: \$NELM\_NETWORK\_PARALLELISM, \$NELM\_RELEASE\_ROLLBACK\_NETWORK\_PARALLELISM


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_RELEASE\_ROLLBACK\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_RELEASE\_ROLLBACK\_LOG\_LEVEL

- `--release-history-limit` (default: `10`)

  Limit the number of releases in release history\. When limit is exceeded the oldest releases are deleted\. Release resources are not affected\. Var: \$NELM\_RELEASE\_HISTORY\_LIMIT

- `--release-storage` (default: `""`)

  How releases should be stored\. Var: \$NELM\_RELEASE\_STORAGE

- `--release-storage-sql-connection` (default: `""`)

  SQL connection string for MySQL release storage driver\. Var: \$NELM\_RELEASE\_STORAGE\_SQL\_CONNECTION

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### release plan install

Plan a release install to Kubernetes\.

**Usage:**

```shell
nelm release plan install [options...] -n namespace -r release [chart-dir]
```

**Options:**

- `--delete-propagation` (default: `"Foreground"`)

  Default delete propagation strategy\. Vars: \$NELM\_DELETE\_PROPAGATION, \$NELM\_RELEASE\_PLAN\_INSTALL\_DELETE\_PROPAGATION

- `--diff-context-lines` (default: `3`)

  Show N lines of context around diffs\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_DIFF\_CONTEXT\_LINES

- `--exit-code` (default: `false`)

  Return exit code 0 if no changes, 1 if error, 2 if any changes planned\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_EXIT\_CODE

- `--force-adoption` (default: `false`)

  Always adopt resources, even if they belong to a different Helm release\. Vars: \$NELM\_FORCE\_ADOPTION, \$NELM\_RELEASE\_PLAN\_INSTALL\_FORCE\_ADOPTION

- `-n`, `--namespace` (default: `""`)

  The release namespace\. Resources with no namespace will be deployed here\. Vars: \$NELM\_NAMESPACE, \$NELM\_RELEASE\_PLAN\_INSTALL\_NAMESPACE

- `--no-install-crds` (default: `false`)

  Don't install CRDs from "crds/" directories of installed charts\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_INSTALL\_CRDS

- `--no-remove-manual-changes` (default: `false`)

  Don't remove fields added manually to the resource in the cluster if fields aren't present in the manifest\. Vars: \$NELM\_NO\_REMOVE\_MANUAL\_CHANGES, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_REMOVE\_MANUAL\_CHANGES

- `--provenance-keyring` (default: `""`)

  Path to keyring containing public keys to verify chart provenance\. Vars: \$NELM\_PROVENANCE\_KEYRING, \$NELM\_RELEASE\_PLAN\_INSTALL\_PROVENANCE\_KEYRING

- `--provenance-strategy` (default: `"never"`)

  Strategy for provenance verifying\. Vars: \$NELM\_PROVENANCE\_STRATEGY, \$NELM\_RELEASE\_PLAN\_INSTALL\_PROVENANCE\_STRATEGY

- `-r`, `--release` (default: `""`)

  The release name\. Must be unique within the release namespace\. Vars: \$NELM\_RELEASE, \$NELM\_RELEASE\_PLAN\_INSTALL\_RELEASE

- `--release-info-annotations` (default: `{}`)

  Add annotations to release metadata\. Vars: \$NELM\_RELEASE\_INFO\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_PLAN\_INSTALL\_RELEASE\_INFO\_ANNOTATIONS\_\*

- `--release-labels` (default: `{}`)

  Add labels to the release\. What kind of labels depends on the storage driver\. Vars: \$NELM\_RELEASE\_LABELS\_\*, \$NELM\_RELEASE\_PLAN\_INSTALL\_RELEASE\_LABELS\_\*

- `--save-graph-to` (default: `""`)

  Save the Graphviz install graph to a file\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_SAVE\_GRAPH\_TO

- `--save-plan` (default: `""`)

  Save the gzip\-compressed JSON install plan to the specified file\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_SAVE\_PLAN

- `--show-insignificant-diffs` (default: `false`)

  Show insignificant diff lines\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_SHOW\_INSIGNIFICANT\_DIFFS

- `--show-sensitive-diffs` (default: `false`)

  Show sensitive diff lines\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_SHOW\_SENSITIVE\_DIFFS

- `--show-verbose-crd-diffs` (default: `false`)

  Show verbose CRD diff lines\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_SHOW\_VERBOSE\_CRD\_DIFFS

- `--show-verbose-diffs` (default: `true`)

  Show verbose diff lines\. Var: \$NELM\_RELEASE\_PLAN\_INSTALL\_SHOW\_VERBOSE\_DIFFS

- `--templates-allow-dns` (default: `false`)

  Allow performing DNS requests in templating\. Vars: \$NELM\_TEMPLATES\_ALLOW\_DNS, \$NELM\_RELEASE\_PLAN\_INSTALL\_TEMPLATES\_ALLOW\_DNS

- `--timeout` (default: `0s`)

  Fail if not finished in time\. Vars: \$NELM\_TIMEOUT, \$NELM\_RELEASE\_PLAN\_INSTALL\_TIMEOUT


**Values options:**

- `--no-default-values` (default: `false`)

  Ignore values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_VALUES, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_DEFAULT\_VALUES

- `--set` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. Vars: \$NELM\_SET, \$NELM\_RELEASE\_PLAN\_INSTALL\_SET

- `--set-file` (default: `[]`)

  Set new values, where the key is the value path and the value is the path to the file with the value content\. Vars: \$NELM\_SET\_FILE, \$NELM\_RELEASE\_PLAN\_INSTALL\_SET\_FILE

- `--set-json` (default: `[]`)

  Set new values, where the key is the value path and the value is JSON\. Vars: \$NELM\_SET\_JSON, \$NELM\_RELEASE\_PLAN\_INSTALL\_SET\_JSON

- `--set-literal` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a literal string\. Vars: \$NELM\_SET\_LITERAL, \$NELM\_RELEASE\_PLAN\_INSTALL\_SET\_LITERAL

- `--set-root-json` (default: `[]`)

  Set new keys in the global context \(\$\), where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_ROOT\_JSON, \$NELM\_RELEASE\_PLAN\_INSTALL\_SET\_ROOT\_JSON

- `--set-runtime-json` (default: `[]`)

  Set new keys in \$\.Runtime, where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_RUNTIME\_JSON, \$NELM\_RELEASE\_PLAN\_INSTALL\_SET\_RUNTIME\_JSON

- `--set-string` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a string\. Vars: \$NELM\_SET\_STRING, \$NELM\_RELEASE\_PLAN\_INSTALL\_SET\_STRING

- `--values` (default: `[]`)

  Additional values files\. Vars: \$NELM\_VALUES, \$NELM\_RELEASE\_PLAN\_INSTALL\_VALUES


**Secret options:**

- `--no-decrypt-secrets` (default: `false`)

  Do not decrypt secrets and secret values, pass them as is\. Vars: \$NELM\_NO\_DECRYPT\_SECRETS, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_DECRYPT\_SECRETS

- `--no-default-secret-values` (default: `false`)

  Ignore secret\-values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_SECRET\_VALUES, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_DEFAULT\_SECRET\_VALUES

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_RELEASE\_PLAN\_INSTALL\_SECRET\_KEY

- `--secret-values` (default: `[]`)

  Secret values files paths\. Vars: \$NELM\_SECRET\_VALUES, \$NELM\_RELEASE\_PLAN\_INSTALL\_SECRET\_VALUES


**Patch options:**

- `--annotations` (default: `{}`)

  Add annotations to all resources\. Vars: \$NELM\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_PLAN\_INSTALL\_ANNOTATIONS\_\*

- `--app-version` (default: `""`)

  Set appVersion of Chart\.yaml\. Vars: \$NELM\_APP\_VERSION, \$NELM\_RELEASE\_PLAN\_INSTALL\_APP\_VERSION

- `--labels` (default: `{}`)

  Add labels to all resources\. Vars: \$NELM\_LABELS\_\*, \$NELM\_RELEASE\_PLAN\_INSTALL\_LABELS\_\*

- `--runtime-annotations` (default: `{}`)

  Add annotations which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_ANNOTATIONS\_\*, \$NELM\_RELEASE\_PLAN\_INSTALL\_RUNTIME\_ANNOTATIONS\_\*

- `--runtime-labels` (default: `{}`)

  Add labels which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_LABELS\_\*, \$NELM\_RELEASE\_PLAN\_INSTALL\_RUNTIME\_LABELS\_\*


**Progress options:**

- `--no-final-tracking` (default: `false`)

  By default disable tracking operations that have no create/update/delete resource operations after them, which are most tracking operations, to speed up the release\. Vars: \$NELM\_NO\_FINAL\_TRACKING, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_FINAL\_TRACKING


**Chart repository options:**

- `--chart-repo-basic-password` (default: `""`)

  Basic auth password to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_PASSWORD, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_BASIC\_PASSWORD

- `--chart-repo-basic-username` (default: `""`)

  Basic auth username to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_USERNAME, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_BASIC\_USERNAME

- `--chart-repo-ca` (default: `""`)

  Path to TLS CA file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CA, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_CA

- `--chart-repo-cert` (default: `""`)

  Path to TLS client cert file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CERT, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_CERT

- `--chart-repo-key` (default: `""`)

  Path to TLS client key file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_KEY, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_KEY

- `--chart-repo-pass-creds` (default: `false`)

  Allow sending chart repository credentials to domains different from the chart repository domain when downloading charts\. Vars: \$NELM\_CHART\_REPO\_PASS\_CREDS, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_PASS\_CREDS

- `--chart-repo-request-timeout` (default: `0s`)

  Set timeout for all requests to chart repository\. Vars: \$NELM\_CHART\_REPO\_REQUEST\_TIMEOUT, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_REQUEST\_TIMEOUT

- `--chart-repo-url` (default: `""`)

  Set URL of chart repo to be used to look for chart\. Vars: \$NELM\_CHART\_REPO\_URL, \$NELM\_RELEASE\_PLAN\_INSTALL\_CHART\_REPO\_URL

- `--insecure-chart-repos` (default: `false`)

  Allow insecure HTTP connections to chart repository\. Vars: \$NELM\_INSECURE\_CHART\_REPOS, \$NELM\_RELEASE\_PLAN\_INSTALL\_INSECURE\_CHART\_REPOS

- `--no-update-chart-repos` (default: `false`)

  Don't update chart repositories index\. Vars: \$NELM\_NO\_UPDATE\_CHART\_REPOS, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_UPDATE\_CHART\_REPOS

- `--no-verify-chart-repos-tls` (default: `false`)

  Don't verify TLS certificates of chart repository\. Vars: \$NELM\_NO\_VERIFY\_CHART\_REPOS\_TLS, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_VERIFY\_CHART\_REPOS\_TLS

- `--oci-chart-repos-creds` (default: `"~/.docker/config.json"`)

  Credentials to access OCI chart repositories\. Vars: \$NELM\_OCI\_CHART\_REPOS\_CREDS, \$NELM\_RELEASE\_PLAN\_INSTALL\_OCI\_CHART\_REPOS\_CREDS


**Kubernetes connection options:**

- `--kube-api-server` (default: `""`)

  Kubernetes API server address\. Vars: \$NELM\_KUBE\_API\_SERVER, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_API\_SERVER

- `--kube-api-server-tls-name` (default: `""`)

  Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server\. Vars: \$NELM\_KUBE\_API\_SERVER\_TLS\_NAME, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_API\_SERVER\_TLS\_NAME

- `--kube-auth-password` (default: `""`)

  Basic auth password for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PASSWORD, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_AUTH\_PASSWORD

- `--kube-auth-provider` (default: `""`)

  Auth provider name for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_AUTH\_PROVIDER

- `--kube-auth-provider-config` (default: `{}`)

  Auth provider config for authentication in Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER\_CONFIG, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_AUTH\_PROVIDER\_CONFIG

- `--kube-auth-username` (default: `""`)

  Basic auth username for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_USERNAME, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_AUTH\_USERNAME

- `--kube-ca` (default: `""`)

  Path to Kubernetes API server TLS CA file\. Vars: \$NELM\_KUBE\_CA, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CA

- `--kube-ca-data` (default: `""`)

  Pass Kubernetes API server TLS CA data\. Vars: \$NELM\_KUBE\_CA\_DATA, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CA\_DATA

- `--kube-cert` (default: `""`)

  Path to PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CERT

- `--kube-cert-data` (default: `""`)

  Pass PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT\_DATA, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CERT\_DATA

- `--kube-config` (default: `[]`)

  Kubeconfig path\(s\)\. If multiple specified, their contents are merged\. Vars: \$KUBECONFIG, \$NELM\_KUBE\_CONFIG\_\*, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CONFIG\_\*

- `--kube-config-base64` (default: `""`)

  Pass Kubeconfig file content encoded as base64\. Vars: \$NELM\_KUBE\_CONFIG\_BASE\_64, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CONFIG\_BASE\_64

- `--kube-context` (default: `""`)

  Use specified Kubeconfig context\. Vars: \$NELM\_KUBE\_CONTEXT, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CONTEXT

- `--kube-context-cluster` (default: `""`)

  Use cluster from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_CLUSTER, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CONTEXT\_CLUSTER

- `--kube-context-user` (default: `""`)

  Use user from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_USER, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_CONTEXT\_USER

- `--kube-impersonate-group` (default: `[]`)

  Sets Impersonate\-Group headers when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_GROUP, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_IMPERSONATE\_GROUP

- `--kube-impersonate-uid` (default: `""`)

  Sets Impersonate\-Uid header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_UID, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_IMPERSONATE\_UID

- `--kube-impersonate-user` (default: `""`)

  Sets Impersonate\-User header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_USER, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_IMPERSONATE\_USER

- `--kube-key` (default: `""`)

  Path to PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_KEY

- `--kube-key-data` (default: `""`)

  Pass PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY\_DATA, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_KEY\_DATA

- `--kube-proxy-url` (default: `""`)

  Proxy URL to use for proxying all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_PROXY\_URL, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_PROXY\_URL

- `--kube-request-timeout` (default: `0s`)

  Timeout for all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_REQUEST\_TIMEOUT, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_REQUEST\_TIMEOUT

- `--kube-token` (default: `""`)

  Bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_TOKEN

- `--kube-token-path` (default: `""`)

  Path to file with bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN\_PATH, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_TOKEN\_PATH

- `--no-verify-kube-tls` (default: `false`)

  Don't verify TLS certificates of Kubernetes API\. Vars: \$NELM\_NO\_VERIFY\_KUBE\_TLS, \$NELM\_RELEASE\_PLAN\_INSTALL\_NO\_VERIFY\_KUBE\_TLS


**Performance options:**

- `--kube-burst-limit` (default: `100`)

  Burst limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_BURST\_LIMIT, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_BURST\_LIMIT

- `--kube-qps-limit` (default: `30`)

  Queries Per Second limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_QPS\_LIMIT, \$NELM\_RELEASE\_PLAN\_INSTALL\_KUBE\_QPS\_LIMIT

- `--network-parallelism` (default: `30`)

  Limit of network\-related tasks to run in parallel\. Vars: \$NELM\_NETWORK\_PARALLELISM, \$NELM\_RELEASE\_PLAN\_INSTALL\_NETWORK\_PARALLELISM


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_RELEASE\_PLAN\_INSTALL\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_RELEASE\_PLAN\_INSTALL\_LOG\_LEVEL

- `--release-storage` (default: `""`)

  How releases should be stored\. Var: \$NELM\_RELEASE\_STORAGE

- `--release-storage-sql-connection` (default: `""`)

  SQL connection string for MySQL release storage driver\. Var: \$NELM\_RELEASE\_STORAGE\_SQL\_CONNECTION

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### release uninstall

Uninstall a Helm Release from Kubernetes\.

**Usage:**

```shell
nelm release uninstall [options...] -n namespace -r release
```

**Options:**

- `--delete-namespace` (default: `false`)

  Delete the release namespace\. Var: \$NELM\_RELEASE\_UNINSTALL\_DELETE\_NAMESPACE

- `-n`, `--namespace` (default: `""`)

  The release namespace\. Resources with no namespace will be deployed here\. Vars: \$NELM\_NAMESPACE, \$NELM\_RELEASE\_UNINSTALL\_NAMESPACE

- `--no-delete-hooks` (default: `false`)

  Do not remove release hooks\. Var: \$NELM\_RELEASE\_UNINSTALL\_NO\_DELETE\_HOOKS

- `-r`, `--release` (default: `""`)

  The release name\. Must be unique within the release namespace\. Vars: \$NELM\_RELEASE, \$NELM\_RELEASE\_UNINSTALL\_RELEASE

- `--timeout` (default: `0s`)

  Fail if not finished in time\. Vars: \$NELM\_TIMEOUT, \$NELM\_RELEASE\_UNINSTALL\_TIMEOUT


**Progress options:**

- `--no-final-tracking` (default: `false`)

  By default disable tracking operations that have no create/update/delete resource operations after them, which are most tracking operations, to speed up the release\. Vars: \$NELM\_NO\_FINAL\_TRACKING, \$NELM\_RELEASE\_UNINSTALL\_NO\_FINAL\_TRACKING

- `--no-pod-logs` (default: `false`)

  Disable Pod logs collection and printing\. Vars: \$NELM\_NO\_POD\_LOGS, \$NELM\_RELEASE\_UNINSTALL\_NO\_POD\_LOGS

- `--no-show-progress` (default: `false`)

  Don't show logs, events and real\-time info about release resources\. Vars: \$NELM\_NO\_SHOW\_PROGRESS, \$NELM\_RELEASE\_UNINSTALL\_NO\_SHOW\_PROGRESS

- `--progress-interval` (default: `5s`)

  How often to print new logs, events and real\-time info about release resources\. Vars: \$NELM\_PROGRESS\_INTERVAL, \$NELM\_RELEASE\_UNINSTALL\_PROGRESS\_INTERVAL

- `--resource-creation-timeout` (default: `0s`)

  Fail if resource creation tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_CREATION\_TIMEOUT, \$NELM\_RELEASE\_UNINSTALL\_RESOURCE\_CREATION\_TIMEOUT

- `--resource-deletion-timeout` (default: `0s`)

  Fail if resource deletion tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_DELETION\_TIMEOUT, \$NELM\_RELEASE\_UNINSTALL\_RESOURCE\_DELETION\_TIMEOUT

- `--resource-readiness-timeout` (default: `0s`)

  Fail if resource readiness tracking did not finish in time\. Vars: \$NELM\_RESOURCE\_READINESS\_TIMEOUT, \$NELM\_RELEASE\_UNINSTALL\_RESOURCE\_READINESS\_TIMEOUT


**Kubernetes connection options:**

- `--kube-api-server` (default: `""`)

  Kubernetes API server address\. Vars: \$NELM\_KUBE\_API\_SERVER, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_API\_SERVER

- `--kube-api-server-tls-name` (default: `""`)

  Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server\. Vars: \$NELM\_KUBE\_API\_SERVER\_TLS\_NAME, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_API\_SERVER\_TLS\_NAME

- `--kube-auth-password` (default: `""`)

  Basic auth password for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PASSWORD, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_AUTH\_PASSWORD

- `--kube-auth-provider` (default: `""`)

  Auth provider name for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_AUTH\_PROVIDER

- `--kube-auth-provider-config` (default: `{}`)

  Auth provider config for authentication in Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER\_CONFIG, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_AUTH\_PROVIDER\_CONFIG

- `--kube-auth-username` (default: `""`)

  Basic auth username for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_USERNAME, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_AUTH\_USERNAME

- `--kube-ca` (default: `""`)

  Path to Kubernetes API server TLS CA file\. Vars: \$NELM\_KUBE\_CA, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CA

- `--kube-ca-data` (default: `""`)

  Pass Kubernetes API server TLS CA data\. Vars: \$NELM\_KUBE\_CA\_DATA, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CA\_DATA

- `--kube-cert` (default: `""`)

  Path to PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CERT

- `--kube-cert-data` (default: `""`)

  Pass PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT\_DATA, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CERT\_DATA

- `--kube-config` (default: `[]`)

  Kubeconfig path\(s\)\. If multiple specified, their contents are merged\. Vars: \$KUBECONFIG, \$NELM\_KUBE\_CONFIG\_\*, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CONFIG\_\*

- `--kube-config-base64` (default: `""`)

  Pass Kubeconfig file content encoded as base64\. Vars: \$NELM\_KUBE\_CONFIG\_BASE\_64, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CONFIG\_BASE\_64

- `--kube-context` (default: `""`)

  Use specified Kubeconfig context\. Vars: \$NELM\_KUBE\_CONTEXT, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CONTEXT

- `--kube-context-cluster` (default: `""`)

  Use cluster from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_CLUSTER, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CONTEXT\_CLUSTER

- `--kube-context-user` (default: `""`)

  Use user from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_USER, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_CONTEXT\_USER

- `--kube-impersonate-group` (default: `[]`)

  Sets Impersonate\-Group headers when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_GROUP, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_IMPERSONATE\_GROUP

- `--kube-impersonate-uid` (default: `""`)

  Sets Impersonate\-Uid header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_UID, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_IMPERSONATE\_UID

- `--kube-impersonate-user` (default: `""`)

  Sets Impersonate\-User header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_USER, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_IMPERSONATE\_USER

- `--kube-key` (default: `""`)

  Path to PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_KEY

- `--kube-key-data` (default: `""`)

  Pass PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY\_DATA, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_KEY\_DATA

- `--kube-proxy-url` (default: `""`)

  Proxy URL to use for proxying all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_PROXY\_URL, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_PROXY\_URL

- `--kube-request-timeout` (default: `0s`)

  Timeout for all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_REQUEST\_TIMEOUT, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_REQUEST\_TIMEOUT

- `--kube-token` (default: `""`)

  Bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_TOKEN

- `--kube-token-path` (default: `""`)

  Path to file with bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN\_PATH, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_TOKEN\_PATH

- `--no-verify-kube-tls` (default: `false`)

  Don't verify TLS certificates of Kubernetes API\. Vars: \$NELM\_NO\_VERIFY\_KUBE\_TLS, \$NELM\_RELEASE\_UNINSTALL\_NO\_VERIFY\_KUBE\_TLS


**Performance options:**

- `--kube-burst-limit` (default: `100`)

  Burst limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_BURST\_LIMIT, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_BURST\_LIMIT

- `--kube-qps-limit` (default: `30`)

  Queries Per Second limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_QPS\_LIMIT, \$NELM\_RELEASE\_UNINSTALL\_KUBE\_QPS\_LIMIT

- `--network-parallelism` (default: `30`)

  Limit of network\-related tasks to run in parallel\. Vars: \$NELM\_NETWORK\_PARALLELISM, \$NELM\_RELEASE\_UNINSTALL\_NETWORK\_PARALLELISM


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_RELEASE\_UNINSTALL\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_RELEASE\_UNINSTALL\_LOG\_LEVEL

- `--release-history-limit` (default: `10`)

  Limit the number of releases in release history\. When limit is exceeded the oldest releases are deleted\. Release resources are not affected\. Var: \$NELM\_RELEASE\_HISTORY\_LIMIT

- `--release-storage` (default: `""`)

  How releases should be stored\. Var: \$NELM\_RELEASE\_STORAGE

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### release list


This command lists all of the releases for a specified namespace \(uses current namespace context if namespace not specified\)\.

By default, it lists only releases that are deployed or failed\. Flags like
'\-\-uninstalled' and '\-\-all' will alter this behavior\. Such flags can be combined:
'\-\-uninstalled \-\-failed'\.

By default, items are sorted alphabetically\. Use the '\-d' flag to sort by
release date\.

If the \-\-filter flag is provided, it will be treated as a filter\. Filters are
regular expressions \(Perl compatible\) that are applied to the list of releases\.
Only items that match the filter will be returned\.

    $ helm list --filter 'ara[a-z]+'
    NAME                UPDATED                                  CHART
    maudlin-arachnid    2020-06-18 14:17:46.125134977 +0000 UTC  alpine-0.1.0

If no results are found, 'helm list' will exit 0, but with no output \(or in
the case of no '\-q' flag, only headers\)\.

By default, up to 256 items may be returned\. To limit this, use the '\-\-max' flag\.
Setting '\-\-max' to 0 will not return all results\. Rather, it will return the
server's default, which may be much higher than 256\. Pairing the '\-\-max'
flag with the '\-\-offset' flag allows you to page through results\.


**Usage:**

```shell
nelm release list [flags]
```

**Other options:**

- `-a`, `--all` (default: `false`)

  show all releases without any filter applied

- `-A`, `--all-namespaces` (default: `false`)

  list releases across all namespaces

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `-d`, `--date` (default: `false`)

  sort by release date

- `--debug` (default: `false`)

  enable verbose output

- `--deployed` (default: `false`)

  show deployed releases\. If no other is specified, this will be automatically enabled

- `--failed` (default: `false`)

  show failed releases

- `-f`, `--filter` (default: `""`)

  a regular expression \(Perl compatible\)\. Any releases that match the expression will be included in the results

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-m`, `--max` (default: `256`)

  maximum number of releases to fetch

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--no-headers` (default: `false`)

  don't print headers when using the default output format

- `--offset` (default: `0`)

  next release index in the list, used to offset from start value

- `-o`, `--output` (default: `table`)

  prints the output in the specified format\. Allowed values: table, json, yaml

- `--pending` (default: `false`)

  show pending releases

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs

- `-r`, `--reverse` (default: `false`)

  reverse the sort order

- `-l`, `--selector` (default: `""`)

  Selector \(label query\) to filter on, supports '=', '==', and '\!='\.\(e\.g\. \-l key1=value1,key2=value2\)\. Works only for secret\(default\) and configmap storage backends\.

- `-q`, `--short` (default: `false`)

  output short \(quiet\) listing format

- `--superseded` (default: `false`)

  show superseded releases

- `--time-format` (default: `""`)

  format time using golang time formatter\. Example: \-\-time\-format "2006\-01\-02 15:04:05Z0700"

- `--uninstalled` (default: `false`)

  show uninstalled releases \(if 'helm uninstall \-\-keep\-history' was used\)

- `--uninstalling` (default: `false`)

  show releases that are currently being uninstalled


### release history


History prints historical revisions for a given release\.

A default maximum of 256 revisions will be returned\. Setting '\-\-max'
configures the maximum length of the revision list returned\.

The historical release set is printed as a formatted table, e\.g:

    $ helm history angry-bird
    REVISION    UPDATED                     STATUS          CHART             APP VERSION     DESCRIPTION
    1           Mon Oct 3 10:15:13 2016     superseded      alpine-0.1.0      1.0             Initial install
    2           Mon Oct 3 10:15:13 2016     superseded      alpine-0.1.0      1.0             Upgraded successfully
    3           Mon Oct 3 10:15:13 2016     superseded      alpine-0.1.0      1.0             Rolled back to 2
    4           Mon Oct 3 10:15:13 2016     deployed        alpine-0.1.0      1.0             Upgraded successfully


**Usage:**

```shell
nelm release history RELEASE_NAME [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--debug` (default: `false`)

  enable verbose output

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `--max` (default: `256`)

  maximum number of revision to include in history

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `-o`, `--output` (default: `table`)

  prints the output in the specified format\. Allowed values: table, json, yaml

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs


### release get

Get information about a deployed release\.

**Usage:**

```shell
nelm release get [options...] -n namespace -r release [revision]
```

**Options:**

- `-n`, `--namespace` (default: `""`)

  The release namespace\. Resources with no namespace will be deployed here\. Vars: \$NELM\_NAMESPACE, \$NELM\_RELEASE\_GET\_NAMESPACE

- `--print-values` (default: `false`)

  Print Values of the last Helm release\. Var: \$NELM\_RELEASE\_GET\_PRINT\_VALUES

- `-r`, `--release` (default: `""`)

  The release name\. Must be unique within the release namespace\. Vars: \$NELM\_RELEASE, \$NELM\_RELEASE\_GET\_RELEASE


**Kubernetes connection options:**

- `--kube-api-server` (default: `""`)

  Kubernetes API server address\. Vars: \$NELM\_KUBE\_API\_SERVER, \$NELM\_RELEASE\_GET\_KUBE\_API\_SERVER

- `--kube-api-server-tls-name` (default: `""`)

  Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server\. Vars: \$NELM\_KUBE\_API\_SERVER\_TLS\_NAME, \$NELM\_RELEASE\_GET\_KUBE\_API\_SERVER\_TLS\_NAME

- `--kube-auth-password` (default: `""`)

  Basic auth password for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PASSWORD, \$NELM\_RELEASE\_GET\_KUBE\_AUTH\_PASSWORD

- `--kube-auth-provider` (default: `""`)

  Auth provider name for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER, \$NELM\_RELEASE\_GET\_KUBE\_AUTH\_PROVIDER

- `--kube-auth-provider-config` (default: `{}`)

  Auth provider config for authentication in Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER\_CONFIG, \$NELM\_RELEASE\_GET\_KUBE\_AUTH\_PROVIDER\_CONFIG

- `--kube-auth-username` (default: `""`)

  Basic auth username for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_USERNAME, \$NELM\_RELEASE\_GET\_KUBE\_AUTH\_USERNAME

- `--kube-ca` (default: `""`)

  Path to Kubernetes API server TLS CA file\. Vars: \$NELM\_KUBE\_CA, \$NELM\_RELEASE\_GET\_KUBE\_CA

- `--kube-ca-data` (default: `""`)

  Pass Kubernetes API server TLS CA data\. Vars: \$NELM\_KUBE\_CA\_DATA, \$NELM\_RELEASE\_GET\_KUBE\_CA\_DATA

- `--kube-cert` (default: `""`)

  Path to PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT, \$NELM\_RELEASE\_GET\_KUBE\_CERT

- `--kube-cert-data` (default: `""`)

  Pass PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT\_DATA, \$NELM\_RELEASE\_GET\_KUBE\_CERT\_DATA

- `--kube-config` (default: `[]`)

  Kubeconfig path\(s\)\. If multiple specified, their contents are merged\. Vars: \$KUBECONFIG, \$NELM\_KUBE\_CONFIG\_\*, \$NELM\_RELEASE\_GET\_KUBE\_CONFIG\_\*

- `--kube-config-base64` (default: `""`)

  Pass Kubeconfig file content encoded as base64\. Vars: \$NELM\_KUBE\_CONFIG\_BASE\_64, \$NELM\_RELEASE\_GET\_KUBE\_CONFIG\_BASE\_64

- `--kube-context` (default: `""`)

  Use specified Kubeconfig context\. Vars: \$NELM\_KUBE\_CONTEXT, \$NELM\_RELEASE\_GET\_KUBE\_CONTEXT

- `--kube-context-cluster` (default: `""`)

  Use cluster from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_CLUSTER, \$NELM\_RELEASE\_GET\_KUBE\_CONTEXT\_CLUSTER

- `--kube-context-user` (default: `""`)

  Use user from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_USER, \$NELM\_RELEASE\_GET\_KUBE\_CONTEXT\_USER

- `--kube-impersonate-group` (default: `[]`)

  Sets Impersonate\-Group headers when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_GROUP, \$NELM\_RELEASE\_GET\_KUBE\_IMPERSONATE\_GROUP

- `--kube-impersonate-uid` (default: `""`)

  Sets Impersonate\-Uid header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_UID, \$NELM\_RELEASE\_GET\_KUBE\_IMPERSONATE\_UID

- `--kube-impersonate-user` (default: `""`)

  Sets Impersonate\-User header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_USER, \$NELM\_RELEASE\_GET\_KUBE\_IMPERSONATE\_USER

- `--kube-key` (default: `""`)

  Path to PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY, \$NELM\_RELEASE\_GET\_KUBE\_KEY

- `--kube-key-data` (default: `""`)

  Pass PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY\_DATA, \$NELM\_RELEASE\_GET\_KUBE\_KEY\_DATA

- `--kube-proxy-url` (default: `""`)

  Proxy URL to use for proxying all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_PROXY\_URL, \$NELM\_RELEASE\_GET\_KUBE\_PROXY\_URL

- `--kube-request-timeout` (default: `0s`)

  Timeout for all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_REQUEST\_TIMEOUT, \$NELM\_RELEASE\_GET\_KUBE\_REQUEST\_TIMEOUT

- `--kube-token` (default: `""`)

  Bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN, \$NELM\_RELEASE\_GET\_KUBE\_TOKEN

- `--kube-token-path` (default: `""`)

  Path to file with bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN\_PATH, \$NELM\_RELEASE\_GET\_KUBE\_TOKEN\_PATH

- `--no-verify-kube-tls` (default: `false`)

  Don't verify TLS certificates of Kubernetes API\. Vars: \$NELM\_NO\_VERIFY\_KUBE\_TLS, \$NELM\_RELEASE\_GET\_NO\_VERIFY\_KUBE\_TLS


**Performance options:**

- `--kube-burst-limit` (default: `100`)

  Burst limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_BURST\_LIMIT, \$NELM\_RELEASE\_GET\_KUBE\_BURST\_LIMIT

- `--kube-qps-limit` (default: `30`)

  Queries Per Second limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_QPS\_LIMIT, \$NELM\_RELEASE\_GET\_KUBE\_QPS\_LIMIT

- `--network-parallelism` (default: `30`)

  Limit of network\-related tasks to run in parallel\. Vars: \$NELM\_NETWORK\_PARALLELISM, \$NELM\_RELEASE\_GET\_NETWORK\_PARALLELISM


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_RELEASE\_GET\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_RELEASE\_GET\_LOG\_LEVEL

- `--output-format` (default: `"yaml"`)

  Result output format\. Vars: \$NELM\_OUTPUT\_FORMAT, \$NELM\_RELEASE\_GET\_OUTPUT\_FORMAT

- `--release-storage` (default: `""`)

  How releases should be stored\. Var: \$NELM\_RELEASE\_STORAGE

- `--release-storage-sql-connection` (default: `""`)

  SQL connection string for MySQL release storage driver\. Var: \$NELM\_RELEASE\_STORAGE\_SQL\_CONNECTION

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### release plan show

Show plan artifact planned changes\.

**Usage:**

```shell
nelm release plan show [options...] plan.json
```

**Options:**

- `--secret-key` (default: `""`)

  Secret key for decrypting the plan artifact\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_RELEASE\_PLAN\_SHOW\_SECRET\_KEY

- `--secret-work-dir` (default: `""`)

  Working directory for secret operations\. Vars: \$NELM\_SECRET\_WORK\_DIR, \$NELM\_RELEASE\_PLAN\_SHOW\_SECRET\_WORK\_DIR

- `--show-insignificant-diffs` (default: `false`)

  Show insignificant diff lines\. Var: \$NELM\_RELEASE\_PLAN\_SHOW\_SHOW\_INSIGNIFICANT\_DIFFS

- `--show-sensitive-diffs` (default: `false`)

  Show sensitive diff lines\. Var: \$NELM\_RELEASE\_PLAN\_SHOW\_SHOW\_SENSITIVE\_DIFFS

- `--show-verbose-crd-diffs` (default: `false`)

  Show verbose CRD diff lines\. Var: \$NELM\_RELEASE\_PLAN\_SHOW\_SHOW\_VERBOSE\_CRD\_DIFFS

- `--show-verbose-diffs` (default: `true`)

  Show verbose diff lines\. Var: \$NELM\_RELEASE\_PLAN\_SHOW\_SHOW\_VERBOSE\_DIFFS


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_RELEASE\_PLAN\_SHOW\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_RELEASE\_PLAN\_SHOW\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  Temporary directory for operation\. Vars: \$NELM\_TEMP\_DIR, \$NELM\_RELEASE\_PLAN\_SHOW\_TEMP\_DIR


### chart lint

Lint a chart\.

**Usage:**

```shell
nelm chart lint [options...] [chart-dir]
```

**Options:**

- `--delete-propagation` (default: `"Foreground"`)

  Default delete propagation strategy\. Vars: \$NELM\_DELETE\_PROPAGATION, \$NELM\_CHART\_LINT\_DELETE\_PROPAGATION

- `--extra-apiversions` (default: `[]`)

  Extra Kubernetes API versions passed to \$\.Capabilities\.APIVersions\. Vars: \$NELM\_EXTRA\_APIVERSIONS\_\*, \$NELM\_CHART\_LINT\_EXTRA\_APIVERSIONS\_\*

- `--force-adoption` (default: `false`)

  Always adopt resources, even if they belong to a different Helm release\. Vars: \$NELM\_FORCE\_ADOPTION, \$NELM\_CHART\_LINT\_FORCE\_ADOPTION

- `--kube-version` (default: `"1.20.0"`)

  Kubernetes version stub for non\-remote mode\. Var: \$NELM\_CHART\_LINT\_KUBE\_VERSION

- `-n`, `--namespace` (default: `"stub-namespace"`)

  The release namespace\. Resources with no namespace will be deployed here\. Vars: \$NELM\_NAMESPACE, \$NELM\_CHART\_LINT\_NAMESPACE

- `--no-remove-manual-changes` (default: `false`)

  Don't remove fields added manually to the resource in the cluster if fields aren't present in the manifest\. Vars: \$NELM\_NO\_REMOVE\_MANUAL\_CHANGES, \$NELM\_CHART\_LINT\_NO\_REMOVE\_MANUAL\_CHANGES

- `--provenance-keyring` (default: `""`)

  Path to keyring containing public keys to verify chart provenance\. Vars: \$NELM\_PROVENANCE\_KEYRING, \$NELM\_CHART\_LINT\_PROVENANCE\_KEYRING

- `--provenance-strategy` (default: `"never"`)

  Strategy for provenance verifying\. Vars: \$NELM\_PROVENANCE\_STRATEGY, \$NELM\_CHART\_LINT\_PROVENANCE\_STRATEGY

- `-r`, `--release` (default: `"stub-release"`)

  The release name\. Must be unique within the release namespace\. Vars: \$NELM\_RELEASE, \$NELM\_CHART\_LINT\_RELEASE

- `--remote` (default: `false`)

  Allow cluster access for additional checks\. Var: \$NELM\_CHART\_LINT\_REMOTE

- `--templates-allow-dns` (default: `false`)

  Allow performing DNS requests in templating\. Vars: \$NELM\_TEMPLATES\_ALLOW\_DNS, \$NELM\_CHART\_LINT\_TEMPLATES\_ALLOW\_DNS


**Values options:**

- `--no-default-values` (default: `false`)

  Ignore values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_VALUES, \$NELM\_CHART\_LINT\_NO\_DEFAULT\_VALUES

- `--set` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. Vars: \$NELM\_SET, \$NELM\_CHART\_LINT\_SET

- `--set-file` (default: `[]`)

  Set new values, where the key is the value path and the value is the path to the file with the value content\. Vars: \$NELM\_SET\_FILE, \$NELM\_CHART\_LINT\_SET\_FILE

- `--set-json` (default: `[]`)

  Set new values, where the key is the value path and the value is JSON\. Vars: \$NELM\_SET\_JSON, \$NELM\_CHART\_LINT\_SET\_JSON

- `--set-literal` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a literal string\. Vars: \$NELM\_SET\_LITERAL, \$NELM\_CHART\_LINT\_SET\_LITERAL

- `--set-root-json` (default: `[]`)

  Set new keys in the global context \(\$\), where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_ROOT\_JSON, \$NELM\_CHART\_LINT\_SET\_ROOT\_JSON

- `--set-runtime-json` (default: `[]`)

  Set new keys in \$\.Runtime, where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_RUNTIME\_JSON, \$NELM\_CHART\_LINT\_SET\_RUNTIME\_JSON

- `--set-string` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a string\. Vars: \$NELM\_SET\_STRING, \$NELM\_CHART\_LINT\_SET\_STRING

- `--values` (default: `[]`)

  Additional values files\. Vars: \$NELM\_VALUES, \$NELM\_CHART\_LINT\_VALUES


**Secret options:**

- `--no-decrypt-secrets` (default: `false`)

  Do not decrypt secrets and secret values, pass them as is\. Vars: \$NELM\_NO\_DECRYPT\_SECRETS, \$NELM\_CHART\_LINT\_NO\_DECRYPT\_SECRETS

- `--no-default-secret-values` (default: `false`)

  Ignore secret\-values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_SECRET\_VALUES, \$NELM\_CHART\_LINT\_NO\_DEFAULT\_SECRET\_VALUES

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_LINT\_SECRET\_KEY

- `--secret-values` (default: `[]`)

  Secret values files paths\. Vars: \$NELM\_SECRET\_VALUES, \$NELM\_CHART\_LINT\_SECRET\_VALUES


**Patch options:**

- `--annotations` (default: `{}`)

  Add annotations to all resources\. Vars: \$NELM\_ANNOTATIONS\_\*, \$NELM\_CHART\_LINT\_ANNOTATIONS\_\*

- `--app-version` (default: `""`)

  Set appVersion of Chart\.yaml\. Vars: \$NELM\_APP\_VERSION, \$NELM\_CHART\_LINT\_APP\_VERSION

- `--labels` (default: `{}`)

  Add labels to all resources\. Vars: \$NELM\_LABELS\_\*, \$NELM\_CHART\_LINT\_LABELS\_\*

- `--runtime-annotations` (default: `{}`)

  Add annotations which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_ANNOTATIONS\_\*, \$NELM\_CHART\_LINT\_RUNTIME\_ANNOTATIONS\_\*

- `--runtime-labels` (default: `{}`)

  Add labels which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_LABELS\_\*, \$NELM\_CHART\_LINT\_RUNTIME\_LABELS\_\*


**Progress options:**

- `--no-final-tracking` (default: `false`)

  By default disable tracking operations that have no create/update/delete resource operations after them, which are most tracking operations, to speed up the release\. Vars: \$NELM\_NO\_FINAL\_TRACKING, \$NELM\_CHART\_LINT\_NO\_FINAL\_TRACKING


**Chart repository options:**

- `--chart-repo-basic-password` (default: `""`)

  Basic auth password to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_PASSWORD, \$NELM\_CHART\_LINT\_CHART\_REPO\_BASIC\_PASSWORD

- `--chart-repo-basic-username` (default: `""`)

  Basic auth username to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_USERNAME, \$NELM\_CHART\_LINT\_CHART\_REPO\_BASIC\_USERNAME

- `--chart-repo-ca` (default: `""`)

  Path to TLS CA file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CA, \$NELM\_CHART\_LINT\_CHART\_REPO\_CA

- `--chart-repo-cert` (default: `""`)

  Path to TLS client cert file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CERT, \$NELM\_CHART\_LINT\_CHART\_REPO\_CERT

- `--chart-repo-key` (default: `""`)

  Path to TLS client key file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_KEY, \$NELM\_CHART\_LINT\_CHART\_REPO\_KEY

- `--chart-repo-pass-creds` (default: `false`)

  Allow sending chart repository credentials to domains different from the chart repository domain when downloading charts\. Vars: \$NELM\_CHART\_REPO\_PASS\_CREDS, \$NELM\_CHART\_LINT\_CHART\_REPO\_PASS\_CREDS

- `--chart-repo-request-timeout` (default: `0s`)

  Set timeout for all requests to chart repository\. Vars: \$NELM\_CHART\_REPO\_REQUEST\_TIMEOUT, \$NELM\_CHART\_LINT\_CHART\_REPO\_REQUEST\_TIMEOUT

- `--chart-repo-url` (default: `""`)

  Set URL of chart repo to be used to look for chart\. Vars: \$NELM\_CHART\_REPO\_URL, \$NELM\_CHART\_LINT\_CHART\_REPO\_URL

- `--insecure-chart-repos` (default: `false`)

  Allow insecure HTTP connections to chart repository\. Vars: \$NELM\_INSECURE\_CHART\_REPOS, \$NELM\_CHART\_LINT\_INSECURE\_CHART\_REPOS

- `--no-update-chart-repos` (default: `false`)

  Don't update chart repositories index\. Vars: \$NELM\_NO\_UPDATE\_CHART\_REPOS, \$NELM\_CHART\_LINT\_NO\_UPDATE\_CHART\_REPOS

- `--no-verify-chart-repos-tls` (default: `false`)

  Don't verify TLS certificates of chart repository\. Vars: \$NELM\_NO\_VERIFY\_CHART\_REPOS\_TLS, \$NELM\_CHART\_LINT\_NO\_VERIFY\_CHART\_REPOS\_TLS

- `--oci-chart-repos-creds` (default: `"~/.docker/config.json"`)

  Credentials to access OCI chart repositories\. Vars: \$NELM\_OCI\_CHART\_REPOS\_CREDS, \$NELM\_CHART\_LINT\_OCI\_CHART\_REPOS\_CREDS


**Kubernetes connection options:**

- `--kube-api-server` (default: `""`)

  Kubernetes API server address\. Vars: \$NELM\_KUBE\_API\_SERVER, \$NELM\_CHART\_LINT\_KUBE\_API\_SERVER

- `--kube-api-server-tls-name` (default: `""`)

  Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server\. Vars: \$NELM\_KUBE\_API\_SERVER\_TLS\_NAME, \$NELM\_CHART\_LINT\_KUBE\_API\_SERVER\_TLS\_NAME

- `--kube-auth-password` (default: `""`)

  Basic auth password for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PASSWORD, \$NELM\_CHART\_LINT\_KUBE\_AUTH\_PASSWORD

- `--kube-auth-provider` (default: `""`)

  Auth provider name for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER, \$NELM\_CHART\_LINT\_KUBE\_AUTH\_PROVIDER

- `--kube-auth-provider-config` (default: `{}`)

  Auth provider config for authentication in Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER\_CONFIG, \$NELM\_CHART\_LINT\_KUBE\_AUTH\_PROVIDER\_CONFIG

- `--kube-auth-username` (default: `""`)

  Basic auth username for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_USERNAME, \$NELM\_CHART\_LINT\_KUBE\_AUTH\_USERNAME

- `--kube-ca` (default: `""`)

  Path to Kubernetes API server TLS CA file\. Vars: \$NELM\_KUBE\_CA, \$NELM\_CHART\_LINT\_KUBE\_CA

- `--kube-ca-data` (default: `""`)

  Pass Kubernetes API server TLS CA data\. Vars: \$NELM\_KUBE\_CA\_DATA, \$NELM\_CHART\_LINT\_KUBE\_CA\_DATA

- `--kube-cert` (default: `""`)

  Path to PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT, \$NELM\_CHART\_LINT\_KUBE\_CERT

- `--kube-cert-data` (default: `""`)

  Pass PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT\_DATA, \$NELM\_CHART\_LINT\_KUBE\_CERT\_DATA

- `--kube-config` (default: `[]`)

  Kubeconfig path\(s\)\. If multiple specified, their contents are merged\. Vars: \$KUBECONFIG, \$NELM\_KUBE\_CONFIG\_\*, \$NELM\_CHART\_LINT\_KUBE\_CONFIG\_\*

- `--kube-config-base64` (default: `""`)

  Pass Kubeconfig file content encoded as base64\. Vars: \$NELM\_KUBE\_CONFIG\_BASE\_64, \$NELM\_CHART\_LINT\_KUBE\_CONFIG\_BASE\_64

- `--kube-context` (default: `""`)

  Use specified Kubeconfig context\. Vars: \$NELM\_KUBE\_CONTEXT, \$NELM\_CHART\_LINT\_KUBE\_CONTEXT

- `--kube-context-cluster` (default: `""`)

  Use cluster from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_CLUSTER, \$NELM\_CHART\_LINT\_KUBE\_CONTEXT\_CLUSTER

- `--kube-context-user` (default: `""`)

  Use user from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_USER, \$NELM\_CHART\_LINT\_KUBE\_CONTEXT\_USER

- `--kube-impersonate-group` (default: `[]`)

  Sets Impersonate\-Group headers when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_GROUP, \$NELM\_CHART\_LINT\_KUBE\_IMPERSONATE\_GROUP

- `--kube-impersonate-uid` (default: `""`)

  Sets Impersonate\-Uid header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_UID, \$NELM\_CHART\_LINT\_KUBE\_IMPERSONATE\_UID

- `--kube-impersonate-user` (default: `""`)

  Sets Impersonate\-User header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_USER, \$NELM\_CHART\_LINT\_KUBE\_IMPERSONATE\_USER

- `--kube-key` (default: `""`)

  Path to PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY, \$NELM\_CHART\_LINT\_KUBE\_KEY

- `--kube-key-data` (default: `""`)

  Pass PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY\_DATA, \$NELM\_CHART\_LINT\_KUBE\_KEY\_DATA

- `--kube-proxy-url` (default: `""`)

  Proxy URL to use for proxying all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_PROXY\_URL, \$NELM\_CHART\_LINT\_KUBE\_PROXY\_URL

- `--kube-request-timeout` (default: `0s`)

  Timeout for all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_REQUEST\_TIMEOUT, \$NELM\_CHART\_LINT\_KUBE\_REQUEST\_TIMEOUT

- `--kube-token` (default: `""`)

  Bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN, \$NELM\_CHART\_LINT\_KUBE\_TOKEN

- `--kube-token-path` (default: `""`)

  Path to file with bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN\_PATH, \$NELM\_CHART\_LINT\_KUBE\_TOKEN\_PATH

- `--no-verify-kube-tls` (default: `false`)

  Don't verify TLS certificates of Kubernetes API\. Vars: \$NELM\_NO\_VERIFY\_KUBE\_TLS, \$NELM\_CHART\_LINT\_NO\_VERIFY\_KUBE\_TLS


**Performance options:**

- `--kube-burst-limit` (default: `100`)

  Burst limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_BURST\_LIMIT, \$NELM\_CHART\_LINT\_KUBE\_BURST\_LIMIT

- `--kube-qps-limit` (default: `30`)

  Queries Per Second limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_QPS\_LIMIT, \$NELM\_CHART\_LINT\_KUBE\_QPS\_LIMIT

- `--network-parallelism` (default: `30`)

  Limit of network\-related tasks to run in parallel\. Vars: \$NELM\_NETWORK\_PARALLELISM, \$NELM\_CHART\_LINT\_NETWORK\_PARALLELISM


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_LINT\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_LINT\_LOG\_LEVEL

- `--release-storage` (default: `""`)

  How releases should be stored\. Var: \$NELM\_RELEASE\_STORAGE

- `--release-storage-sql-connection` (default: `""`)

  SQL connection string for MySQL release storage driver\. Var: \$NELM\_RELEASE\_STORAGE\_SQL\_CONNECTION

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart render

Render a chart\.

**Usage:**

```shell
nelm chart render [options...] [chart-dir]
```

**Options:**

- `--extra-apiversions` (default: `[]`)

  Extra Kubernetes API versions passed to \$\.Capabilities\.APIVersions\. Vars: \$NELM\_EXTRA\_APIVERSIONS\_\*, \$NELM\_CHART\_RENDER\_EXTRA\_APIVERSIONS\_\*

- `--force-adoption` (default: `false`)

  Always adopt resources, even if they belong to a different Helm release\. Vars: \$NELM\_FORCE\_ADOPTION, \$NELM\_CHART\_RENDER\_FORCE\_ADOPTION

- `--kube-version` (default: `"1.20.0"`)

  Kubernetes version stub for non\-remote mode\. Var: \$NELM\_CHART\_RENDER\_KUBE\_VERSION

- `-n`, `--namespace` (default: `"stub-namespace"`)

  The release namespace\. Resources with no namespace will be deployed here\. Vars: \$NELM\_NAMESPACE, \$NELM\_CHART\_RENDER\_NAMESPACE

- `--provenance-keyring` (default: `""`)

  Path to keyring containing public keys to verify chart provenance\. Vars: \$NELM\_PROVENANCE\_KEYRING, \$NELM\_CHART\_RENDER\_PROVENANCE\_KEYRING

- `--provenance-strategy` (default: `"never"`)

  Strategy for provenance verifying\. Vars: \$NELM\_PROVENANCE\_STRATEGY, \$NELM\_CHART\_RENDER\_PROVENANCE\_STRATEGY

- `-r`, `--release` (default: `"stub-release"`)

  The release name\. Must be unique within the release namespace\. Vars: \$NELM\_RELEASE, \$NELM\_CHART\_RENDER\_RELEASE

- `--remote` (default: `false`)

  Allow cluster access to retrieve Kubernetes version, capabilities and other dynamic data\. Var: \$NELM\_CHART\_RENDER\_REMOTE

- `--save-output-to` (default: `""`)

  Save output with rendered manifests to a file\. Var: \$NELM\_CHART\_RENDER\_SAVE\_OUTPUT\_TO

- `--show-crds` (default: `false`)

  Show CRDs from "crds/" directories in the output\. Var: \$NELM\_CHART\_RENDER\_SHOW\_CRDS

- `--show-only` (default: `[]`)

  Show manifests only from specified template files\. The render result has corresponding template paths specified before each resource manifest\. Var: \$NELM\_CHART\_RENDER\_SHOW\_ONLY\_\*

- `--templates-allow-dns` (default: `false`)

  Allow performing DNS requests in templating\. Vars: \$NELM\_TEMPLATES\_ALLOW\_DNS, \$NELM\_CHART\_RENDER\_TEMPLATES\_ALLOW\_DNS


**Values options:**

- `--no-default-values` (default: `false`)

  Ignore values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_VALUES, \$NELM\_CHART\_RENDER\_NO\_DEFAULT\_VALUES

- `--set` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. Vars: \$NELM\_SET, \$NELM\_CHART\_RENDER\_SET

- `--set-file` (default: `[]`)

  Set new values, where the key is the value path and the value is the path to the file with the value content\. Vars: \$NELM\_SET\_FILE, \$NELM\_CHART\_RENDER\_SET\_FILE

- `--set-json` (default: `[]`)

  Set new values, where the key is the value path and the value is JSON\. Vars: \$NELM\_SET\_JSON, \$NELM\_CHART\_RENDER\_SET\_JSON

- `--set-literal` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a literal string\. Vars: \$NELM\_SET\_LITERAL, \$NELM\_CHART\_RENDER\_SET\_LITERAL

- `--set-root-json` (default: `[]`)

  Set new keys in the global context \(\$\), where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_ROOT\_JSON, \$NELM\_CHART\_RENDER\_SET\_ROOT\_JSON

- `--set-runtime-json` (default: `[]`)

  Set new keys in \$\.Runtime, where the key is the value path and the value is JSON\. This is meant to be generated inside the program, so use \-\-set\-json instead, unless you know what you are doing\. Vars: \$NELM\_SET\_RUNTIME\_JSON, \$NELM\_CHART\_RENDER\_SET\_RUNTIME\_JSON

- `--set-string` (default: `[]`)

  Set new values, where the key is the value path and the value is the value\. The value will always become a string\. Vars: \$NELM\_SET\_STRING, \$NELM\_CHART\_RENDER\_SET\_STRING

- `--values` (default: `[]`)

  Additional values files\. Vars: \$NELM\_VALUES, \$NELM\_CHART\_RENDER\_VALUES


**Secret options:**

- `--no-decrypt-secrets` (default: `false`)

  Do not decrypt secrets and secret values, pass them as is\. Vars: \$NELM\_NO\_DECRYPT\_SECRETS, \$NELM\_CHART\_RENDER\_NO\_DECRYPT\_SECRETS

- `--no-default-secret-values` (default: `false`)

  Ignore secret\-values\.yaml of the top\-level chart\. Vars: \$NELM\_NO\_DEFAULT\_SECRET\_VALUES, \$NELM\_CHART\_RENDER\_NO\_DEFAULT\_SECRET\_VALUES

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_RENDER\_SECRET\_KEY

- `--secret-values` (default: `[]`)

  Secret values files paths\. Vars: \$NELM\_SECRET\_VALUES, \$NELM\_CHART\_RENDER\_SECRET\_VALUES


**Patch options:**

- `--annotations` (default: `{}`)

  Add annotations to all resources\. Vars: \$NELM\_ANNOTATIONS\_\*, \$NELM\_CHART\_RENDER\_ANNOTATIONS\_\*

- `--app-version` (default: `""`)

  Set appVersion of Chart\.yaml\. Vars: \$NELM\_APP\_VERSION, \$NELM\_CHART\_RENDER\_APP\_VERSION

- `--labels` (default: `{}`)

  Add labels to all resources\. Vars: \$NELM\_LABELS\_\*, \$NELM\_CHART\_RENDER\_LABELS\_\*

- `--runtime-annotations` (default: `{}`)

  Add annotations which will not trigger resource updates to all resources\. Vars: \$NELM\_RUNTIME\_ANNOTATIONS\_\*, \$NELM\_CHART\_RENDER\_RUNTIME\_ANNOTATIONS\_\*


**Chart repository options:**

- `--chart-repo-basic-password` (default: `""`)

  Basic auth password to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_PASSWORD, \$NELM\_CHART\_RENDER\_CHART\_REPO\_BASIC\_PASSWORD

- `--chart-repo-basic-username` (default: `""`)

  Basic auth username to authenticate in chart repository\. Vars: \$NELM\_CHART\_REPO\_BASIC\_USERNAME, \$NELM\_CHART\_RENDER\_CHART\_REPO\_BASIC\_USERNAME

- `--chart-repo-ca` (default: `""`)

  Path to TLS CA file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CA, \$NELM\_CHART\_RENDER\_CHART\_REPO\_CA

- `--chart-repo-cert` (default: `""`)

  Path to TLS client cert file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_CERT, \$NELM\_CHART\_RENDER\_CHART\_REPO\_CERT

- `--chart-repo-key` (default: `""`)

  Path to TLS client key file for connecting to chart repository\. Vars: \$NELM\_CHART\_REPO\_KEY, \$NELM\_CHART\_RENDER\_CHART\_REPO\_KEY

- `--chart-repo-pass-creds` (default: `false`)

  Allow sending chart repository credentials to domains different from the chart repository domain when downloading charts\. Vars: \$NELM\_CHART\_REPO\_PASS\_CREDS, \$NELM\_CHART\_RENDER\_CHART\_REPO\_PASS\_CREDS

- `--chart-repo-request-timeout` (default: `0s`)

  Set timeout for all requests to chart repository\. Vars: \$NELM\_CHART\_REPO\_REQUEST\_TIMEOUT, \$NELM\_CHART\_RENDER\_CHART\_REPO\_REQUEST\_TIMEOUT

- `--chart-repo-url` (default: `""`)

  Set URL of chart repo to be used to look for chart\. Vars: \$NELM\_CHART\_REPO\_URL, \$NELM\_CHART\_RENDER\_CHART\_REPO\_URL

- `--insecure-chart-repos` (default: `false`)

  Allow insecure HTTP connections to chart repository\. Vars: \$NELM\_INSECURE\_CHART\_REPOS, \$NELM\_CHART\_RENDER\_INSECURE\_CHART\_REPOS

- `--no-update-chart-repos` (default: `false`)

  Don't update chart repositories index\. Vars: \$NELM\_NO\_UPDATE\_CHART\_REPOS, \$NELM\_CHART\_RENDER\_NO\_UPDATE\_CHART\_REPOS

- `--no-verify-chart-repos-tls` (default: `false`)

  Don't verify TLS certificates of chart repository\. Vars: \$NELM\_NO\_VERIFY\_CHART\_REPOS\_TLS, \$NELM\_CHART\_RENDER\_NO\_VERIFY\_CHART\_REPOS\_TLS

- `--oci-chart-repos-creds` (default: `"~/.docker/config.json"`)

  Credentials to access OCI chart repositories\. Vars: \$NELM\_OCI\_CHART\_REPOS\_CREDS, \$NELM\_CHART\_RENDER\_OCI\_CHART\_REPOS\_CREDS


**Kubernetes connection options:**

- `--kube-api-server` (default: `""`)

  Kubernetes API server address\. Vars: \$NELM\_KUBE\_API\_SERVER, \$NELM\_CHART\_RENDER\_KUBE\_API\_SERVER

- `--kube-api-server-tls-name` (default: `""`)

  Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server\. Vars: \$NELM\_KUBE\_API\_SERVER\_TLS\_NAME, \$NELM\_CHART\_RENDER\_KUBE\_API\_SERVER\_TLS\_NAME

- `--kube-auth-password` (default: `""`)

  Basic auth password for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PASSWORD, \$NELM\_CHART\_RENDER\_KUBE\_AUTH\_PASSWORD

- `--kube-auth-provider` (default: `""`)

  Auth provider name for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER, \$NELM\_CHART\_RENDER\_KUBE\_AUTH\_PROVIDER

- `--kube-auth-provider-config` (default: `{}`)

  Auth provider config for authentication in Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_PROVIDER\_CONFIG, \$NELM\_CHART\_RENDER\_KUBE\_AUTH\_PROVIDER\_CONFIG

- `--kube-auth-username` (default: `""`)

  Basic auth username for Kubernetes API\. Vars: \$NELM\_KUBE\_AUTH\_USERNAME, \$NELM\_CHART\_RENDER\_KUBE\_AUTH\_USERNAME

- `--kube-ca` (default: `""`)

  Path to Kubernetes API server TLS CA file\. Vars: \$NELM\_KUBE\_CA, \$NELM\_CHART\_RENDER\_KUBE\_CA

- `--kube-ca-data` (default: `""`)

  Pass Kubernetes API server TLS CA data\. Vars: \$NELM\_KUBE\_CA\_DATA, \$NELM\_CHART\_RENDER\_KUBE\_CA\_DATA

- `--kube-cert` (default: `""`)

  Path to PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT, \$NELM\_CHART\_RENDER\_KUBE\_CERT

- `--kube-cert-data` (default: `""`)

  Pass PEM\-encoded TLS client cert for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_CERT\_DATA, \$NELM\_CHART\_RENDER\_KUBE\_CERT\_DATA

- `--kube-config` (default: `[]`)

  Kubeconfig path\(s\)\. If multiple specified, their contents are merged\. Vars: \$KUBECONFIG, \$NELM\_KUBE\_CONFIG\_\*, \$NELM\_CHART\_RENDER\_KUBE\_CONFIG\_\*

- `--kube-config-base64` (default: `""`)

  Pass Kubeconfig file content encoded as base64\. Vars: \$NELM\_KUBE\_CONFIG\_BASE\_64, \$NELM\_CHART\_RENDER\_KUBE\_CONFIG\_BASE\_64

- `--kube-context` (default: `""`)

  Use specified Kubeconfig context\. Vars: \$NELM\_KUBE\_CONTEXT, \$NELM\_CHART\_RENDER\_KUBE\_CONTEXT

- `--kube-context-cluster` (default: `""`)

  Use cluster from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_CLUSTER, \$NELM\_CHART\_RENDER\_KUBE\_CONTEXT\_CLUSTER

- `--kube-context-user` (default: `""`)

  Use user from Kubeconfig for current context\. Vars: \$NELM\_KUBE\_CONTEXT\_USER, \$NELM\_CHART\_RENDER\_KUBE\_CONTEXT\_USER

- `--kube-impersonate-group` (default: `[]`)

  Sets Impersonate\-Group headers when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_GROUP, \$NELM\_CHART\_RENDER\_KUBE\_IMPERSONATE\_GROUP

- `--kube-impersonate-uid` (default: `""`)

  Sets Impersonate\-Uid header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_UID, \$NELM\_CHART\_RENDER\_KUBE\_IMPERSONATE\_UID

- `--kube-impersonate-user` (default: `""`)

  Sets Impersonate\-User header when authenticating in Kubernetes\. Vars: \$NELM\_KUBE\_IMPERSONATE\_USER, \$NELM\_CHART\_RENDER\_KUBE\_IMPERSONATE\_USER

- `--kube-key` (default: `""`)

  Path to PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY, \$NELM\_CHART\_RENDER\_KUBE\_KEY

- `--kube-key-data` (default: `""`)

  Pass PEM\-encoded TLS client key for connecting to Kubernetes API\. Vars: \$NELM\_KUBE\_KEY\_DATA, \$NELM\_CHART\_RENDER\_KUBE\_KEY\_DATA

- `--kube-proxy-url` (default: `""`)

  Proxy URL to use for proxying all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_PROXY\_URL, \$NELM\_CHART\_RENDER\_KUBE\_PROXY\_URL

- `--kube-request-timeout` (default: `0s`)

  Timeout for all requests to Kubernetes API\. Vars: \$NELM\_KUBE\_REQUEST\_TIMEOUT, \$NELM\_CHART\_RENDER\_KUBE\_REQUEST\_TIMEOUT

- `--kube-token` (default: `""`)

  Bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN, \$NELM\_CHART\_RENDER\_KUBE\_TOKEN

- `--kube-token-path` (default: `""`)

  Path to file with bearer token for authentication in Kubernetes\. Vars: \$NELM\_KUBE\_TOKEN\_PATH, \$NELM\_CHART\_RENDER\_KUBE\_TOKEN\_PATH

- `--no-verify-kube-tls` (default: `false`)

  Don't verify TLS certificates of Kubernetes API\. Vars: \$NELM\_NO\_VERIFY\_KUBE\_TLS, \$NELM\_CHART\_RENDER\_NO\_VERIFY\_KUBE\_TLS


**Performance options:**

- `--kube-burst-limit` (default: `100`)

  Burst limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_BURST\_LIMIT, \$NELM\_CHART\_RENDER\_KUBE\_BURST\_LIMIT

- `--kube-qps-limit` (default: `30`)

  Queries Per Second limit for requests to Kubernetes\. Vars: \$NELM\_KUBE\_QPS\_LIMIT, \$NELM\_CHART\_RENDER\_KUBE\_QPS\_LIMIT

- `--network-parallelism` (default: `30`)

  Limit of network\-related tasks to run in parallel\. Vars: \$NELM\_NETWORK\_PARALLELISM, \$NELM\_CHART\_RENDER\_NETWORK\_PARALLELISM


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_RENDER\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_RENDER\_LOG\_LEVEL

- `--release-storage` (default: `""`)

  How releases should be stored\. Var: \$NELM\_RELEASE\_STORAGE

- `--release-storage-sql-connection` (default: `""`)

  SQL connection string for MySQL release storage driver\. Var: \$NELM\_RELEASE\_STORAGE\_SQL\_CONNECTION

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart download


Retrieve a package from a package repository, and download it locally\.

This is useful for fetching packages to inspect, modify, or repackage\. It can
also be used to perform cryptographic verification of a chart without installing
the chart\.

There are options for unpacking the chart after download\. This will create a
directory for the chart and uncompress into that directory\.

If the \-\-verify flag is specified, the requested chart MUST have a provenance
file, and MUST pass the verification process\. Failure in any part of this will
result in an error, and the chart will not be saved locally\.


**Usage:**

```shell
nelm chart download [chart URL | repo/chartname] [...] [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--ca-file` (default: `""`)

  verify certificates of HTTPS\-enabled servers using this CA bundle

- `--cert-file` (default: `""`)

  identify HTTPS client using this SSL certificate file

- `--debug` (default: `false`)

  enable verbose output

- `-d`, `--destination` (default: `"."`)

  location to write the chart\. If this and untardir are specified, untardir is appended to this

- `--devel` (default: `false`)

  use development versions, too\. Equivalent to version '\>0\.0\.0\-0'\. If \-\-version is set, this is ignored\.

- `--insecure-skip-tls-verify` (default: `false`)

  skip tls certificate checks for the chart download

- `--key-file` (default: `""`)

  identify HTTPS client using this SSL key file

- `--keyring` (default: `"~/.gnupg/pubring.gpg"`)

  location of public keys used for verification

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--pass-credentials` (default: `false`)

  pass credentials to all domains

- `--password` (default: `""`)

  chart repository password where to locate the requested chart

- `--plain-http` (default: `false`)

  use insecure HTTP connections for the chart download

- `--prov` (default: `false`)

  fetch the provenance file, but don't perform verification

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repo` (default: `""`)

  chart repository url where to locate the requested chart

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs

- `--untar` (default: `false`)

  if set to true, will untar the chart after downloading it

- `--untardir` (default: `"."`)

  if untar is specified, this flag specifies the name of the directory into which the chart is expanded

- `--username` (default: `""`)

  chart repository username where to locate the requested chart

- `--verify` (default: `false`)

  verify the package before using it

- `--version` (default: `""`)

  specify a version constraint for the chart version to use\. This constraint can be a specific tag \(e\.g\. 1\.1\.1\) or it may reference a valid range \(e\.g\. ^2\.0\.0\)\. If this is not specified, the latest version is used


### chart upload


Upload a chart to a registry\.

If the chart has an associated provenance file,
it will also be uploaded\.


**Usage:**

```shell
nelm chart upload [archive] [remote] [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--ca-file` (default: `""`)

  verify certificates of HTTPS\-enabled servers using this CA bundle

- `--cert-file` (default: `""`)

  identify registry client using this SSL certificate file

- `--debug` (default: `false`)

  enable verbose output

- `--insecure-skip-tls-verify` (default: `false`)

  skip tls certificate checks for the chart upload

- `--key-file` (default: `""`)

  identify registry client using this SSL key file

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--plain-http` (default: `false`)

  use insecure HTTP connections for the chart upload

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs


### chart pack


This command packages a chart into a versioned chart archive file\. If a path
is given, this will look at that path for a chart \(which must contain a
Chart\.yaml file\) and then package that directory\.

Versioned chart archives are used by Helm package repositories\.

To sign a chart, use the '\-\-sign' flag\. In most cases, you should also
provide '\-\-keyring path/to/secret/keys' and '\-\-key keyname'\.

  \$ nelm chart pack \-\-sign \./mychart \-\-key mykey \-\-keyring \~/\.gnupg/secring\.gpg

If '\-\-keyring' is not specified, Helm usually defaults to the public keyring
unless your environment is otherwise configured\.


**Usage:**

```shell
nelm chart pack [CHART_PATH] [...] [flags]
```

**Other options:**

- `--app-version` (default: `""`)

  set the appVersion on the chart to this version

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--debug` (default: `false`)

  enable verbose output

- `-u`, `--dependency-update` (default: `false`)

  update dependencies from "Chart\.yaml" to dir "charts/" before packaging

- `-d`, `--destination` (default: `"."`)

  location to write the chart\.

- `--key` (default: `""`)

  name of the key to use when signing\. Used if \-\-sign is true

- `--keyring` (default: `"~/.gnupg/pubring.gpg"`)

  location of a public keyring

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--passphrase-file` (default: `""`)

  location of a file which contains the passphrase for the signing key\. Use "\-" in order to read from stdin\.

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs

- `--sign` (default: `false`)

  use a PGP private key to sign this package

- `--version` (default: `""`)

  set the version on the chart to this semver version


### chart init

Initialize a new chart in the specified directory\. If PATH is not specified, uses the current directory\.

**Usage:**

```shell
nelm chart init [PATH]
```

**Options:**

- `--ts` (default: `false`)

  Initialize TypeScript chart\. Var: \$NELM\_CHART\_INIT\_TS


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_INIT\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_INIT\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret key create

Create a new chart secret key\.

**Usage:**

```shell
nelm chart secret key create [options...]
```

**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_KEY\_CREATE\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_KEY\_CREATE\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret key rotate

Decrypt with an old secret key, then encrypt with a new secret key chart files secret\-values\.yaml and secret/\*\.

**Usage:**

```shell
nelm chart secret key rotate [options...] --old-secret-key secret-key --new-secret-key secret-key [chart-dir]
```

**Options:**

- `--new-secret-key` (default: `""`)

  New secret key\. Var: \$NELM\_CHART\_SECRET\_KEY\_ROTATE\_NEW\_SECRET\_KEY

- `--old-secret-key` (default: `""`)

  Old secret key\. Var: \$NELM\_CHART\_SECRET\_KEY\_ROTATE\_OLD\_SECRET\_KEY

- `--secret-values` (default: `[]`)

  Secret values files paths\. Vars: \$NELM\_SECRET\_VALUES, \$NELM\_CHART\_SECRET\_KEY\_ROTATE\_SECRET\_VALUES


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_KEY\_ROTATE\_COLOR\_MODE

- `--log-level` (default: `"info"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_KEY\_ROTATE\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret values-file edit

Interactively edit encrypted values file\.

**Usage:**

```shell
nelm chart secret values-file edit [options...] --secret-key secret-key values-file
```

**Options:**

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_EDIT\_SECRET\_KEY


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_EDIT\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_EDIT\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret values-file encrypt

Encrypt values file and print result to stdout\.

**Usage:**

```shell
nelm chart secret values-file encrypt [options...] --secret-key secret-key values-file
```

**Options:**

- `--save-output-to` (default: `""`)

  Save encrypted output to a file\. Var: \$NELM\_CHART\_SECRET\_VALUES\_FILE\_ENCRYPT\_SAVE\_OUTPUT\_TO

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_ENCRYPT\_SECRET\_KEY


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_ENCRYPT\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_ENCRYPT\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret values-file decrypt

Decrypt values file and print result to stdout\.

**Usage:**

```shell
nelm chart secret values-file decrypt [options...] --secret-key secret-key values-file
```

**Options:**

- `--save-output-to` (default: `""`)

  Save decrypted output to a file\. Var: \$NELM\_CHART\_SECRET\_VALUES\_FILE\_DECRYPT\_SAVE\_OUTPUT\_TO

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_DECRYPT\_SECRET\_KEY


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_DECRYPT\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_VALUES\_FILE\_DECRYPT\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret file edit

Interactively edit encrypted file\.

**Usage:**

```shell
nelm chart secret file edit [options...] --secret-key secret-key file
```

**Options:**

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_SECRET\_FILE\_EDIT\_SECRET\_KEY


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_FILE\_EDIT\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_FILE\_EDIT\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret file encrypt

Encrypt file and print result to stdout\.

**Usage:**

```shell
nelm chart secret file encrypt [options...] --secret-key secret-key file
```

**Options:**

- `--save-output-to` (default: `""`)

  Save encrypted output to a file\. Var: \$NELM\_CHART\_SECRET\_FILE\_ENCRYPT\_SAVE\_OUTPUT\_TO

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_SECRET\_FILE\_ENCRYPT\_SECRET\_KEY


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_FILE\_ENCRYPT\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_FILE\_ENCRYPT\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart secret file decrypt

Decrypt file and print result to stdout\.

**Usage:**

```shell
nelm chart secret file decrypt [options...] --secret-key secret-key file
```

**Options:**

- `--save-output-to` (default: `""`)

  Save decrypted output to a file\. Var: \$NELM\_CHART\_SECRET\_FILE\_DECRYPT\_SAVE\_OUTPUT\_TO

- `--secret-key` (default: `""`)

  Secret key\. Vars: \$NELM\_SECRET\_KEY, \$NELM\_CHART\_SECRET\_FILE\_DECRYPT\_SECRET\_KEY


**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_CHART\_SECRET\_FILE\_DECRYPT\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_CHART\_SECRET\_FILE\_DECRYPT\_LOG\_LEVEL

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


### chart dependency download

Download chart dependencies from Chart\.lock\.

**Usage:**

```shell
nelm chart dependency download CHART [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--debug` (default: `false`)

  enable verbose output

- `--keyring` (default: `"~/.gnupg/pubring.gpg"`)

  keyring containing public keys

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs

- `--skip-refresh` (default: `false`)

  do not refresh the local repository cache

- `--verify` (default: `false`)

  verify the packages against signatures


### chart dependency update


Update the on\-disk dependencies to mirror Chart\.yaml\.

This command verifies that the required charts, as expressed in 'Chart\.yaml',
are present in 'charts/' and are at an acceptable version\. It will pull down
the latest charts that satisfy the dependencies, and clean up old dependencies\.

On successful update, this will generate a lock file that can be used to
rebuild the dependencies to an exact version\.

Dependencies are not required to be represented in 'Chart\.yaml'\. For that
reason, an update command will not remove charts unless they are \(a\) present
in the Chart\.yaml file, but \(b\) at the wrong version\.


**Usage:**

```shell
nelm chart dependency update CHART [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--debug` (default: `false`)

  enable verbose output

- `--keyring` (default: `"~/.gnupg/pubring.gpg"`)

  keyring containing public keys

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs

- `--skip-refresh` (default: `false`)

  do not refresh the local repository cache

- `--verify` (default: `false`)

  verify the packages against signatures


### repo add

Set up a new chart repository\.

**Usage:**

```shell
nelm repo add [NAME] [URL] [flags]
```

**Other options:**

- `--allow-deprecated-repos` (default: `false`)

  by default, this command will not allow adding official repos that have been permanently deleted\. This disables that behavior

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--ca-file` (default: `""`)

  verify certificates of HTTPS\-enabled servers using this CA bundle

- `--cert-file` (default: `""`)

  identify HTTPS client using this SSL certificate file

- `--debug` (default: `false`)

  enable verbose output

- `--force-update` (default: `false`)

  replace \(overwrite\) the repo if it already exists

- `--insecure-skip-tls-verify` (default: `false`)

  skip tls certificate checks for the repository

- `--key-file` (default: `""`)

  identify HTTPS client using this SSL key file

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--no-update` (default: `false`)

  Ignored\. Formerly, it would disabled forced updates\. It is deprecated by force\-update\.

- `--pass-credentials` (default: `false`)

  pass credentials to all domains

- `--password` (default: `""`)

  chart repository password

- `--password-stdin` (default: `false`)

  read chart repository password from stdin

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs

- `--username` (default: `""`)

  chart repository username


### repo remove

Remove a chart repository\.

**Usage:**

```shell
nelm repo remove [REPO1 [REPO2 ...]] [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--debug` (default: `false`)

  enable verbose output

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs


### repo update

Update info about available charts for all chart repositories\.

**Usage:**

```shell
nelm repo update [REPO1 [REPO2 ...]] [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--debug` (default: `false`)

  enable verbose output

- `--fail-on-repo-update-fail` (default: `false`)

  update fails if any of the repository updates fail

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs


### repo login

Log in to an OCI registry with charts\.

**Usage:**

```shell
nelm repo login [host] [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--ca-file` (default: `""`)

  verify certificates of HTTPS\-enabled servers using this CA bundle

- `--cert-file` (default: `""`)

  identify registry client using this SSL certificate file

- `--debug` (default: `false`)

  enable verbose output

- `--insecure` (default: `false`)

  allow connections to TLS registry without certs

- `--key-file` (default: `""`)

  identify registry client using this SSL key file

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `-p`, `--password` (default: `""`)

  registry password or identity token

- `--password-stdin` (default: `false`)

  read password or identity token from stdin

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs

- `-u`, `--username` (default: `""`)

  registry username


### repo logout

Log out from an OCI registry with charts\.

**Usage:**

```shell
nelm repo logout [host] [flags]
```

**Other options:**

- `--burst-limit` (default: `100`)

  client\-side default throttling limit

- `--debug` (default: `false`)

  enable verbose output

- `--kube-apiserver` (default: `""`)

  the address and the port for the Kubernetes API server

- `--kube-as-group` (default: `[]`)

  group to impersonate for the operation, this flag can be repeated to specify multiple groups\.

- `--kube-as-user` (default: `""`)

  username to impersonate for the operation

- `--kube-ca-file` (default: `""`)

  the certificate authority file for the Kubernetes API server connection

- `--kube-context` (default: `""`)

  name of the kubeconfig context to use

- `--kube-insecure-skip-tls-verify` (default: `false`)

  if true, the Kubernetes API server's certificate will not be checked for validity\. This will make your HTTPS connections insecure

- `--kube-tls-server-name` (default: `""`)

  server name to use for Kubernetes API server certificate validation\. If it is not provided, the hostname used to contact the server is used

- `--kube-token` (default: `""`)

  bearer token used for authentication

- `--kubeconfig` (default: `""`)

  path to the kubeconfig file

- `-n`, `--namespace` (default: `""`)

  namespace scope for this request

- `--qps` (default: `0`)

  queries per second used when communicating with the Kubernetes API, not including bursting

- `--registry-config` (default: `"~/.config/helm/registry/config.json"`)

  path to the registry config file

- `--repository-cache` (default: `"~/.cache/helm/repository"`)

  path to the file containing cached repository indexes

- `--repository-config` (default: `"~/.config/helm/repositories.yaml"`)

  path to the file containing repository names and URLs


### completion bash

Generate the autocompletion script for the bash shell\.

This script depends on the 'bash\-completion' package\.
If it is not installed already, you can install it via your OS's package manager\.

To load completions in your current shell session:

	source <(nelm completion bash)

To load completions for every new session, execute once:

\#\#\#\# Linux:

	nelm completion bash > /etc/bash_completion.d/nelm

\#\#\#\# macOS:

	nelm completion bash > $(brew --prefix)/etc/bash_completion.d/nelm

You will need to start a new shell for this setup to take effect\.


**Usage:**

```shell
nelm completion bash
```

**Other options:**

- `--no-descriptions` (default: `false`)

  disable completion descriptions


### completion fish

Generate the autocompletion script for the fish shell\.

To load completions in your current shell session:

	nelm completion fish | source

To load completions for every new session, execute once:

	nelm completion fish > ~/.config/fish/completions/nelm.fish

You will need to start a new shell for this setup to take effect\.


**Usage:**

```shell
nelm completion fish [flags]
```

**Other options:**

- `--no-descriptions` (default: `false`)

  disable completion descriptions


### completion powershell

Generate the autocompletion script for powershell\.

To load completions in your current shell session:

	nelm completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile\.


**Usage:**

```shell
nelm completion powershell [flags]
```

**Other options:**

- `--no-descriptions` (default: `false`)

  disable completion descriptions


### completion zsh

Generate the autocompletion script for the zsh shell\.

If shell completion is not already enabled in your environment you will need
to enable it\.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(nelm completion zsh)

To load completions for every new session, execute once:

\#\#\#\# Linux:

	nelm completion zsh > "${fpath[1]}/_nelm"

\#\#\#\# macOS:

	nelm completion zsh > $(brew --prefix)/share/zsh/site-functions/_nelm

You will need to start a new shell for this setup to take effect\.


**Usage:**

```shell
nelm completion zsh [flags]
```

**Other options:**

- `--no-descriptions` (default: `false`)

  disable completion descriptions


### version

Show version\.

**Usage:**

```shell
nelm version [options...]
```

**Other options:**

- `--color-mode` (default: `"auto"`)

  Color mode for logs\. Allowed: auto, off, on\. Vars: \$NELM\_COLOR\_MODE, \$NELM\_VERSION\_COLOR\_MODE

- `--log-level` (default: `"error"`)

  Set log level\. Allowed: silent, error, warning, info, debug, trace\. Vars: \$NELM\_LOG\_LEVEL, \$NELM\_VERSION\_LOG\_LEVEL

- `--output-format` (default: `"yaml"`)

  Result output format\. Vars: \$NELM\_OUTPUT\_FORMAT, \$NELM\_VERSION\_OUTPUT\_FORMAT

- `--temp-dir` (default: `""`)

  The directory for temporary files\. By default, create a new directory in the default system directory for temporary files\. Var: \$NELM\_TEMP\_DIR


## Feature Gates

Feature gates are experimental features that can be enabled via environment variables.

### NELM_FEAT_REMOTE_CHARTS

**Default:** `false`

Allow not only local, but also remote charts as an argument to cli commands\. Also adds the "\-\-chart\-version" option

### NELM_FEAT_NATIVE_RELEASE_LIST

**Default:** `false`

Use the native "release list" command instead of "helm list" exposed as "release list"

### NELM_FEAT_PERIODIC_STACK_TRACES

**Default:** `false`

Print stack traces periodically to help with debugging deadlocks and other issues

### NELM_FEAT_NATIVE_RELEASE_UNINSTALL

**Default:** `false`

Use the new "release uninstall" command implementation \(not fully backwards compatible\)

### NELM_FEAT_FIELD_SENSITIVE

**Default:** `false`

Enable JSONPath\-based selective sensitive field redaction

### NELM_FEAT_PREVIEW_V2

**Default:** `false`

Activate all feature gates that will be enabled by default in Nelm v2

### NELM_FEAT_CLEAN_NULL_FIELDS

**Default:** `false`

Enable cleaning of null fields from resource manifests for better Helm chart compatibility

### NELM_FEAT_MORE_DETAILED_EXIT_CODE_FOR_PLAN

**Default:** `false`

Make the "plan" command with the flag "\-\-exit\-code" return an exit code 3 instead of 2 when no resource changes, but still must install the release

### NELM_FEAT_RESOURCE_VALIDATION

**Default:** `false`

Validate chart resources against specific Kubernetes resources' schemas

### NELM_FEAT_TYPESCRIPT

**Default:** `false`

Enable TypeScript chart rendering from ts/ directory

