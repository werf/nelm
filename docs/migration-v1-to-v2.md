# Migrating from Nelm v1 to v2

Migrate in three stages:

1. [**Before upgrading**](#before-upgrading-find-problems-with-v2-keep-installing-with-v1): run Nelm v2 on your charts to find what needs changing, and fix it in a form Nelm v1 also accepts. Production keeps installing with v1. Prepare the script changes that work only in v2.
2. [**Upgrading**](#upgrading): switch to v2 in a fixed order. The new secret and chart formats can't be undone by switching back.
3. [**After upgrading**](#after-upgrading): check the defaults that changed and remove what you no longer need.

werf users: follow the [werf migration guide](https://werf.io/docs/v3/resources/migration_from_v2_to_v3.html) instead.

## Checklist

**Before upgrading (v1 still installs):**

- **Everyone:** [run v2 to find problems](#find-problems-with-v2) and [fix what it reports](#fix-what-v2-reports).

Then check the [changes v2 doesn't report](#changes-v2-doesnt-report) that apply to you:

- **Charts:** [protect sensitive fields in diffs, update templates and annotations](#charts-templates-and-annotations), [check values and chart files](#charts-values-and-patches).

And [prepare scripts](#prepare-scripts):

- **Scripts:** [prepare v2 commands and flags](#scripts-commands-and-flags), [accept the new output formats and exit codes](#scripts-output-formats-and-exit-codes).

**Upgrading:**

- **Everyone:** [switch in this order](#order-of-the-switch), then [check the first install](#first-install).
- **SQL release storage:** [run the storage migration with a privileged role](#release-storage) before other v2 jobs use it.
- **TypeScript charts:** [move to Node.js](#typescript-charts).
- **Secrets:** [upgrade all readers before writing secrets](#secrets).
- **Packed charts:** [upgrade consumers before packing](#packed-charts).

**After upgrading:**

- **Everyone:** [check the changed defaults](#check-the-changed-defaults), [clean up](#clean-up).

## Before upgrading: find problems with v2, keep installing with v1

In this stage, production keeps using Nelm v1. You run v2 only to find problems, and fix each one in a form that v1 also accepts, so the fixes can be merged one by one. The sections below say how for each change.

### Find problems with v2

1. Install Nelm v2 next to v1: on your machine or in a separate CI job.
2. Run these v2 commands for each chart and each environment, with the same release name, namespace, values and secret values flags as your installs:

   ```shell
   nelm chart render ./chart --values values.yaml --set image.tag=1.2.3
   nelm chart lint ./chart --values values.yaml --set image.tag=1.2.3
   ```

   They don't install anything and don't need the cluster. Without `-n`, `{{ .Release.Namespace }}` renders as `stub-namespace`. Pass the secret key as usual. `nelm chart render` prints Secret data: run it where the output isn't shared, or write it to a private file with `--save-output-to`.
3. v2 may stop at the first error. Fix it using the [table below](#fix-what-v2-reports), and run again until the commands pass. Also read the warnings they print. If v2 rejects a flag (`unknown flag`), use its replacement from [Scripts: commands and flags](#scripts-commands-and-flags).
4. Check each fix with v1 too (`nelm chart render`, your tests) before merging it: production still installs with v1.
5. Compare `nelm chart render` output of v1 and v2 with the same flags, for each environment: [values priority](#charts-values-and-patches) and other silent changes show up only there.

Then go through the [changes v2 doesn't report](#changes-v2-doesnt-report), and read [Check the changed defaults](#check-the-changed-defaults) before switching.

**Don't run v2 `release` commands against production release storage before [the switch](#order-of-the-switch):** v2 migrates SQL release storage if you use it, and a v2 install is the switch itself.

### Fix what v2 reports

All these fixes also work in v1.

| v2 reports | Fix |
|---|---|
| A number or boolean in annotations or labels | Quote it: `example.com/replicas: "3"` or `{{ .Values.replicas | quote }}`. v1 dropped such values with a warning, so the first install adds them to live resources: remove the ones you don't need. |
| Values or a schema rejected by `values.schema.json` validation | Fix the values or the schema: v1 could hide these errors. Schemas without `$schema` are now checked as JSON Schema 2020-12; add `"$schema": "http://json-schema.org/draft-07/schema#"` to keep draft-07. The error text changed: update scripts that parse it. `--no-values-schema-validation` skips the check temporarily in v2; v1 doesn't have the flag. |

### Changes v2 doesn't report

The commands above don't reliably catch these changes, so check the ones that apply to your charts.

#### Charts: templates and annotations

**Runtime annotations and labels.** `--runtime-annotations` now appear in `chart render` output. Changing only the values of `--runtime-annotations` or `--runtime-labels` no longer updates live resources. Use `--annotations` and `--labels` for metadata whose change alone must update live resources.

**External dependencies.** v2 ignores `<name>.external-dependency.werf.io/resource` and `/namespace` annotations, with a warning. Add `werf.io/deploy-dependency-<name>` **without** `external=` next to them, and remove the old ones after switching:

```yaml
annotations:
  # Used by v1. Remove after switching.
  secret.external-dependency.werf.io/resource: secret/my-vault-secret
  secret.external-dependency.werf.io/namespace: vault
  # Used by v2.
  werf.io/deploy-dependency-secret: state=present,kind=Secret,version=v1,name=my-vault-secret,namespace=vault
```

- v1 ignores the new annotation if no release resource matches it; v2 waits for the resource in the cluster. v2 warns about the old annotations on every command until you remove them.
- Set `kind`, `version` and `name`; `group` for resources outside the core API group; and `namespace` for namespaced resources (`{{ .Release.Namespace }}` for the release namespace): otherwise a release resource with the same name in another namespace also matches.
- `state=present` waits for the resource to exist, as before; `state=ready` also waits for readiness.
- After switching, you can add `external=true` so that only the cluster is searched. After you remove the old annotations, the next install may still warn about them once, because the previous release has them.
- The one-key form `<name>.external-dependency.werf.io: <apiVersion>:<kind>[:<namespace>]:<name>` is replaced the same way.

**`<name>.dependency.werf.io`** is now ignored without a warning. Replace it with `werf.io/deploy-dependency-<name>`, which v1 understands too, and add `external=false` together with switching (v1 rejects it):

```yaml
# Before
db.dependency.werf.io: apps/v1:StatefulSet:postgres
# After
werf.io/deploy-dependency-db: state=present,group=apps,version=v1,kind=StatefulSet,name=postgres,namespace={{ .Release.Namespace }}
```

**Annotation keys like `werf-io/weight` or `helm-sh/hook`** worked in v1 as `werf.io/weight` and `helm.sh/hook`. They are now ordinary annotations: rename them to the real keys.

**Explicit `null` values** are now removed from manifests. Set a real value where a missing field means something else to Kubernetes or a CRD.

**Sensitive fields in diffs.** For `Secret` resources and resources annotated `werf.io/sensitive: "true"`, v1 diffs hid all values; v2 hides only `data.*` and `stringData.*`. **Before the first v2 plan**, list every field to hide in `werf.io/sensitive-paths`, including `$.data.*` and `$.stringData.*` if needed: the annotation replaces the default list. It works in v1 too, where it immediately replaces hiding the whole resource. Check the first plans in private output, and don't use `--show-sensitive-diffs` where logs are shared.

```yaml
annotations:
  werf.io/sensitive-paths: "$.spec.token,$.data.*,$.stringData.*"
```

**Deletion propagation** is now `Background`: Nelm doesn't wait for dependents of a deleted resource, such as its Pods. If a later step needs them gone, set `werf.io/delete-propagation: Foreground` on the resource (works in v1 too), or `--delete-propagation=Foreground` for all resources.

**Hooks on uninstall.** v1 deleted hooks on uninstall unless `--no-delete-hooks` was passed; v2 never deletes them, and the flag is removed. Where hooks must go with the release, set `werf.io/ownership: release` on them or delete them separately.

**`.Capabilities`.** `.Capabilities.HelmVersion` reports Helm 4: update templates that check it. Without cluster access, `chart render` and `chart lint` assume Kubernetes `1.36.0` instead of `1.20.0`; pass `--kube-version` if templates depend on it.

#### Charts: values and patches

**Keys set in both secret and non-secret values.** In v1, the chart's `secret-values.yaml` and `--secret-values` overrode `--values` and all `--set*`. v2 orders values as documented, from lowest to highest: `values.yaml`, the chart's `secret-values.yaml`, `--values`, `--secret-values`, `--set-json`, `--set`, `--set-string`, `--set-file`, `--set-literal`. To keep the secret value on both versions, remove the same key from `--values` and `--set*`.

**Decrypted secrets in the release storage.** v2 stores the decrypted `--secret-values` in the release like other values, and so do subcharts' `secret-values.yaml` and the secret values of packed charts: `release get --print-values` shows them, and changing only `--secret-values` creates a new revision. Before switching, restrict access to the release storage (Secrets, ConfigMaps or SQL) and to `release get` output as you do for the secrets themselves.

**`export-values` and `import-values`** now also apply to `--values`, `--secret-values` and `--set*`. A subchart key you set directly now loses to an exported parent value from the same or a higher source: `--set backend.config.greeting=x` loses to an exported `--set backendConfig.greeting=y`. Set the parent key instead, or remove the export.

**`patches.yaml`** in the root of a chart or subchart is now read as resource patches. Rename a file with this name if it's for something else, or pass `--no-default-patches` in v2.

**`.werf-data/`** in the chart root is now read as the recorded inputs of a packed chart and is never excluded by `.helmignore`. Rename an unrelated directory with this name.

### Prepare scripts

Removed commands and flags fail only when scripts run v2, so prepare these changes in advance.

#### Scripts: commands and flags

Most new commands and flags don't exist in v1. Prepare the new calls now and switch them together with the binary. A removed flag is an argument error in v2, even if it did nothing before.

**Commands:**

| v1 | v2 |
|---|---|
| `nelm repo <subcommand>` | `nelm chart repo <subcommand>` |
| `nelm release history NAME --max N -o json` | `nelm release history -r NAME -n NAMESPACE --revisions-limit N --output-format json` |
| `nelm release list -o json` | `nelm release list -n NAMESPACE --output-format json`. Without `-n`, all namespaces are listed. |
| `nelm chart secret key rotate` | Re-encrypt each file: see [Secrets](#secrets) |

**`release list`** now shows the latest revision of every release, whatever its status; Helm's `list` showed only `deployed` and `failed` releases by default. Its Helm flags are removed (see below): filter the JSON output by status and name, or use `--release-label-selector`.

**Kubernetes connection flags and variables** of `release list` and `release history`:

| v1 | v2 |
|---|---|
| `--kubeconfig` | `--kube-config` (`KUBECONFIG` is still read) |
| `--kube-apiserver` | `--kube-api-server` |
| `--kube-as-user`, `--kube-as-group` | `--kube-impersonate-user`, `--kube-impersonate-group` |
| `--kube-ca-file` | `--kube-ca` |
| `--kube-tls-server-name` | `--kube-api-server-tls-name` |
| `--kube-insecure-skip-tls-verify` | `--no-verify-kube-tls` |
| `--burst-limit`, `--qps` | `--kube-burst-limit`, `--kube-qps-limit` |
| `--debug`, `HELM_DEBUG` | `--log-level debug` |
| `HELM_KUBECONTEXT`, `HELM_KUBETOKEN`, `HELM_NAMESPACE` | `NELM_KUBE_CONTEXT`, `NELM_KUBE_TOKEN`, `NELM_NAMESPACE` |
| `HELM_DRIVER`, `HELM_DRIVER_SQL_CONNECTION_STRING` | `--release-storage` (`NELM_RELEASE_STORAGE`), `--release-storage-sql-connection` (`NELM_RELEASE_STORAGE_SQL_CONNECTION`). Without them, v2 doesn't find releases stored in ConfigMaps or SQL and creates new ones in Secrets. |
| `--registry-config`, `--repository-config`, `--repository-cache` | Removed |

**`chart pack` and `chart upload`:**

| Command | v1 flag | v2 flag |
|---|---|---|
| `chart pack` | `--version` | `--chart-version` |
| `chart pack` | `--key`, `--keyring`, `--passphrase-file` | `--sign-key`, `--sign-keyring`, `--sign-passphrase-file` |
| `chart upload` | `--ca-file`, `--cert-file`, `--key-file` | `--chart-repo-ca`, `--chart-repo-cert`, `--chart-repo-key` |
| `chart upload` | `--insecure-skip-tls-verify`, `--plain-http` | `--no-verify-chart-repos-tls`, `--insecure-chart-repos` |

- Remove their Kubernetes connection flags and Helm registry and repository config flags. For a registry credentials file, use `--oci-chart-repos-creds FILE`.
- `chart pack` takes one chart directory: run it once per chart.
- `chart pack` now records `--values`, `--secret-values`, `--set*` and `--patches` in the chart, see [Packed charts](#packed-charts). Packing an already packed chart adds to what's recorded, and keeps `defaultValuesDisable` or `defaultSecretValuesDisable` (under `values`) and `defaultPatchesDisable` recorded in `.werf-data/metadata.yaml`: remove the key to turn the default back on, or use `--reset-values` to start over.
- `chart upload` refuses a tag equal to the tag of an image listed in the chart's `helm.sh/images` annotation in the same repository. Other images and charts in the repository aren't checked: don't reuse their tags.

**`--set-runtime-json`** is removed, and v2 no longer sets `.Runtime`. Switch it together with the binary, and choose one of the replacements:

```shell
# v1. Template: {{ $.Runtime.ci.job }}
--set-runtime-json 'ci={"job":"42"}'

# v2, templates unchanged (works in subcharts too). Template: {{ $.Runtime.ci.job }}
--set-root-json 'Runtime.ci={"job":"42"}'

# v2, data in values. Template: {{ $.Values.ci.job }}
--set-json 'ci={"job":"42"}'
```

Switch `--set-root-json 'Runtime...'` together with the binary: v1 replaces `.Runtime` with its own. For the `--set-json` option only: values are stored in the release, so don't put data there that mustn't be stored; allow the new key if `values.schema.json` forbids additional properties; it reaches a subchart only under `global` or the subchart's key, such as `backend.ci`.

**Other removed flags:**

| Flag | What to do |
|---|---|
| Helm flags of `release list`: `-a`/`--all`, `-A`/`--all-namespaces`, `-d`/`--date`, `--deployed`, `--failed`, `--pending`, `--superseded`, `--uninstalled`, `--uninstalling`, `-f`/`--filter`, `-m`/`--max`, `--offset`, `--no-headers`, `-o`/`--output`, `-r`/`--reverse`, `-l`/`--selector`, `-q`/`--short`, `--time-format` | Remove. Use `--output-format json` and filter the output, or `--release-label-selector` |
| `--show-verbose-diffs` | Remove when switching; diffs of created and deleted resources are always shown, except CRDs (`--show-verbose-crd-diffs`) |
| `--force-adoption` on `chart render` | Remove now: it had no effect there |
| `--save-rollback-graph-to` | `--save-graph-to` on `release rollback`; remove it from `release install` |
| `--no-delete-hooks` (`NELM_RELEASE_UNINSTALL_NO_DELETE_HOOKS`) on `release uninstall` | See [Hooks on uninstall](#charts-templates-and-annotations) |
| `--local-resource-validation`, `--resource-validation-kube-version`, `--resource-validation-schema` | Remove; use `--resource-validation-extra-schema` for extra schemas |
| `--no-update`, `--fail-on-repo-update-fail` on `chart repo` | Remove; `repo update` fails if any repository fails |
| `--deno-binary-path` (`NELM_DENO_BINARY_PATH`) | `--node-binary-path` (`NELM_NODE_BINARY_PATH`) on rendering commands; remove elsewhere. See [TypeScript charts](#typescript-charts) |

**Helm directory variables.** `HELM_CONFIG_HOME`, `HELM_CACHE_HOME`, `HELM_DATA_HOME` and `HELM_REPOSITORY_*` are no longer read when rendering and installing. Their `helm/` directories are now under `XDG_CONFIG_HOME`, `XDG_CACHE_HOME` and `XDG_DATA_HOME` (by default `~/.config`, `~/.cache` and `~/.local/share` on Linux). Put `repositories.yaml` in `$XDG_CONFIG_HOME/helm/` (`~/Library/Preferences/helm/` on macOS by default), or add the repositories again. Unset `HELM_REPOSITORY_CONFIG`, `HELM_REPOSITORY_CACHE` and `HELM_CONTENT_CACHE`: `nelm chart repo` still uses them and would write where installs don't read.

#### Scripts: output formats and exit codes

Update parsers to accept both formats before switching.

| Output | What changes |
|---|---|
| `release list --output-format json` | An `{apiVersion: v2, releases: [...]}` object instead of Helm's array (or the `apiVersion: v1` object with `NELM_FEAT_NATIVE_RELEASE_LIST`). `releases` can be empty. |
| `release history --output-format json` | An `{apiVersion: v1, releases: [...]}` object with the same release fields. |
| Release fields in both | `revision` is a number. `chart` is `{name, version, appVersion}`: read `chart.appVersion` instead of `app_version`, and `deployedAt.human` instead of `updated`. `chart` and `deployedAt` can be `null`. |
| `release get` | `apiVersion: v3`. Hooks and resources are in one list: read manifests from `.resourceSpecs[].unstruct`, find hooks by `.resourceSpecs[].resourceMeta.annotations["helm.sh/hook"]`. `.release.storageLabels` is now `.release.labels`. |
| Reports of `--save-report-to` | `"apiVersion": "v3"` instead of `"version": 3`. |
| `chart render` | CRDs first, then by kind, group, version, namespace and name. Includes `--runtime-annotations`. No longer adds empty `annotations: {}` and `labels: {}`. Regenerate saved copies you compare it with. |
| Diffs | Diffs of created and deleted resources are always shown, except CRDs (`--show-verbose-crd-diffs`). Sensitive resources show their diff with only `data.*`/`stringData.*` (or `werf.io/sensitive-paths`) hidden. |

**`release get` output** includes rendered manifests with Secret data: don't save it where logs are shared. With `--print-values` it adds `.values`: the chart defaults merged with the stored values, including decrypted `--secret-values`. Get them with `--output-format json | jq .values`.

**`werf.io/release-version`** is now added to releases, including existing revisions when v2 first updates them. If you set this annotation yourself to a value other than `v1`, move your data to another key before upgrading.

**`release plan install --exit-code`** can return `3`: no resource changes, but the release still has to be installed or updated. Don't treat it as an error or skip the install.

| Code | Meaning |
|---|---|
| `0` | No changes. |
| `1` | Error. |
| `2` | Resource changes are planned. |
| `3` | Only the release needs to be installed or updated. |

## Upgrading

### Order of the switch

Some effects of v2 stay if you switch back to v1. Switch in this order:

1. **Protect rollback history.** If you installed with `release install --use-plan`, v1 ignored `--release-history-limit` and may have kept all revisions. The first v2 command that creates a revision of such a release removes old revisions down to the limit (10 by default). To keep more for a rollback, raise `--release-history-limit` in every install and rollback job before switching them.
2. **SQL release storage**, if used: [run the storage migration](#release-storage) before any v2 job uses it.
3. **Upgrade everything that reads secrets or installs packed charts:** developer machines, CI jobs, jobs that apply saved plans. Switch the prepared script changes at the same time. v2 requires macOS 12 or newer, or Linux kernel 3.2 or newer.
4. **Only then pack charts with v2 and edit secrets:** see [Packed charts](#packed-charts) and [Secrets](#secrets).

Plans saved by v1 are rejected by v2: run `release plan install --save-plan PATH` again. Plans contain the release values, including decrypted secret values, and are encrypted only if the secret key is passed: store them like secrets.

### First install

Before the first `release install` with v2:

1. If CI logs are shared, make sure [sensitive fields are hidden](#charts-templates-and-annotations).
2. Compare `chart render` of v1 and v2. Values priority, `export-values`, `patches.yaml`, removed `null` values and Helm 4 can change manifests without an error.
3. Review `release plan install`. Created and deleted CRDs show a full diff only with `--show-verbose-crd-diffs`.

Releases whose stored values change, for example releases installed with `--secret-values`, get one new revision.

### Release storage

If releases are stored in SQL (`--release-storage=sql`), v2 adds a `nelm_release_annotations_v1` table and records migrations in `nelm_migrations`. Every v2 command that opens the storage runs the migration, including `release plan install` and `release list`.

1. Before any v2 job uses the storage, run `nelm release list` against it once with a role that can create tables and reference `releases_v1`. Don't create the tables by hand: Nelm would try to create them again.
2. Row-level security is enabled on `nelm_release_annotations_v1` without policies, so other roles can't read its rows. Create RLS policies for every role that needs these rows, including read-only roles, matching those for `releases_v1`.

### TypeScript charts

TypeScript charts are now rendered with Node.js instead of Deno and are always enabled. v2 can't render a chart built for Deno, so prepare these steps in a v2 branch and switch them together with the binary.

1. **Install Node.js 26 or newer** wherever a chart with `ts/src` is rendered or installed. The `-node` Nelm images (`ghcr.io/werf/nelm:2-stable-node` and others) include it. If `node` isn't in `PATH`, pass `--node-binary-path`.
2. **Move the chart to `@nelm/chart-ts-sdk` 0.2.0:** replace `ts/deno.json` with `ts/package.json` and `ts/tsconfig.json` as `nelm chart ts init` creates them, and move the dependencies.
3. **Generate and commit a lock file** with npm, pnpm, yarn or bun (`deno.lock` doesn't count). A clean build of the chart (`chart ts build`, `chart pack`) needs that package manager in `PATH` to install dependencies. Installing a built chart needs neither the package manager nor the lock file, only Node.js.
4. **Rebuild `ts/dist/bundle.js`** and pack charts again: charts built for Deno don't render.
5. **Pack a test chart** and install it with v2 before switching production.

`NELM_FEAT_TYPESCRIPT` is no longer needed: a chart with `ts/src/index.ts` or `ts/src/index.js` is rendered even if the gate wasn't set in v1. Remove `ts/` if you don't use it.

### Secrets

v2 encrypts secrets with authenticated AES-GCM. **Nelm v1 and werf v2 can't read secrets written by Nelm v2, and v2 can't write the old format.** v2 reads old secrets.

1. Upgrade every environment that reads the secrets.
2. Only then create, edit or rotate secrets. After that, don't use Nelm v1 or werf v2 with these secrets: switching the binary back doesn't restore the old format.

**New secret values keep their type.** Values entered as unquoted numbers or booleans reach templates as numbers or booleans: quote them if templates expect strings. Existing encrypted values stay strings; to give one a type, enter it again with `nelm chart secret values-file edit`.

**Don't put secrets in comments:** comments on encrypted values are stored in cleartext.

**Rotating the key.** `nelm chart secret key rotate` is removed. Decrypt each secret file of the chart, its subcharts and extra secret values files with the old key (`nelm chart secret values-file decrypt`, `nelm chart secret file decrypt` with `--secret-key`), then encrypt it with the new key (`values-file encrypt`, `file encrypt`).

### Packed charts

`nelm chart pack` now records `--values`, `--secret-values` (encrypted), `--set*` and `--patches` in the `.werf-data/` directory of the chart, and `release install` applies them as if you passed them again. werf v3 publishes bundles in the same format.

Nelm 1.31.3 and earlier and werf 2.84 and earlier ignore `.werf-data/`: they install such charts **without the recorded values**, or fail later, without saying why; werf 2.85 and later v2 releases, and Nelm v1 releases after 1.31.3, refuse them. Before the first `chart pack` with v2:

1. Upgrade every Nelm and werf that installs, plans, renders or lints packed charts, including charts that depend on them.
2. Pack a test chart with a value used in a template, and check in each environment that the value reaches the rendered manifests.

`--no-default-values` and `--no-default-secret-values` of `release install` don't disable values recorded at pack time. Helm, Argo CD and Flux don't read `.werf-data/`: pass `.werf-data/values.yaml` and `.werf-data/extra-values.yaml` (if present) as values files; they still don't get the recorded `--set*`, secret values and patches.

## After upgrading

### Check the changed defaults

| What | v1 | v2 | What to check |
|---|---|---|---|
| Resource validation | Off unless `NELM_FEAT_RESOURCE_VALIDATION=true`. | Resources are checked against schemas of Kubernetes and popular CRDs embedded in the binary, in `release install`, `release plan install`, `release rollback` and `chart lint`. | Fix rejected resources. Skip some with `--resource-validation-skip` (for example `kind=MyKind`), add schemas with `--resource-validation-extra-schema`, or turn validation off with `--no-resource-validation`. |
| Readiness of custom resources | `Ready`/`Available` conditions weren't recognized, so the resource was ready at once. | Nelm waits for the condition to become `True`. | If a resource never sets it, fix its controller, or set `werf.io/track-termination-mode: NonBlocking` on it. This doesn't make it ready: a `state=ready` dependency on it still waits. |
| Dependencies on resources outside the release | A `werf.io/deploy-dependency-*` or `delete-dependency-*` annotation that matched no release resource was ignored. | If it has `kind`, `version` and `name`, Nelm waits for the resource in the cluster. | Add `external=false` to keep ignoring it, or remove the annotation. |
| `release uninstall` | Helm's uninstall unless `NELM_FEAT_NATIVE_RELEASE_UNINSTALL` was set. | The native uninstall based on a plan. | Uninstall a test release first and check which resources and hooks remain, the deletion order and tracking. `--delete-propagation`, `--save-graph-to` and `--save-report-to` help to tune and inspect it. |
| OCI charts | Pulled by tag. | Pulled by one manifest digest; provenance, if verified, is checked against it. | To verify provenance, pass `--provenance-strategy=verify` and `--provenance-keyring`. For registries with a self-signed certificate, pass the CA with `--chart-repo-ca` (`--ca-file` for `chart download`). |
| Helm engine | Helm 3. | Helm 4.2. | Compare `chart render` output: template functions, merging of values and chart loading follow Helm 4. |

### Clean up

- Remove the variables of removed gates: `NELM_FEAT_PREVIEW_V2`, `NELM_FEAT_RESOURCE_VALIDATION`, `NELM_FEAT_NATIVE_RELEASE_LIST`, `NELM_FEAT_NATIVE_RELEASE_UNINSTALL`, `NELM_FEAT_CASE_INSENSITIVE_CONDITION_TRACKING`, `NELM_FEAT_REMOTE_CHARTS`, `NELM_FEAT_FIELD_SENSITIVE`, `NELM_FEAT_CLEAN_NULL_FIELDS`, `NELM_FEAT_MORE_DETAILED_EXIT_CODE_FOR_PLAN`, `NELM_FEAT_TYPESCRIPT`. Their behavior is always on; setting them only prints a warning. Keep `NELM_FEAT_PERIODIC_STACK_TRACES` and `NELM_FEAT_ADOPT_DECKHOUSE_CONTROLLER_FIELDS` if you use them.
- Remove the old external dependency annotations you kept for v1.
