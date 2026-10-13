# Release annotations and `release get` migration

Release annotations can now be read from a metadata-only release list without fetching the release body. The `Annotations(ctx)` getter keeps its signature.

- New revisions store annotations in Secret/ConfigMap metadata or a separate SQL table, not in `info.annotations` in the stored body. Older revisions remain readable from their bodies; there is no automatic backfill.
- Every new record includes `nelm.werf.io/released-by: "nelm"`. It is returned by get, list and history. This key is reserved; an explicit different value is rejected.
- For Secret/ConfigMap storage, annotation keys must satisfy Kubernetes rules, and all keys and values together, including the service annotation, must fit within 256 KiB. Invalid annotations are rejected before pruning history.
- Before using the new version with non-DDL SQL roles, initialize storage once with a database owner or migration role to apply the new schema. Nelm migrations are tracked in `nelm_migrations`, separate from Helm migrations. Reading annotations follows release visibility; writing them requires access under the release row's UPDATE permissions.
- SQL ignores stored annotations when their saved release modification time differs from the current release modification time and reads the body instead. This comparison has one-second precision.
- Writing with an older Nelm or Helm after switching to this format is unsupported: it can discard metadata annotations or leave stale SQL annotations. Annotations removed this way cannot always be recovered.
- Updating only annotations still does not trigger a deployment when the release is otherwise up to date.

## Library and output changes

Replace `action.ReleaseGetResultV2` with `action.ReleaseGetResultV3`, and `.Release.StorageLabels` with `.Release.Labels`. JSON/YAML consumers must use `apiVersion: "v3"` and `release.labels` instead of `release.storageLabels`. The labels themselves are unchanged. List remains v2 and history remains v1.

The release v1/v2 `Info.Annotations` fields remain available in memory, plans and public output. Storage restores them from metadata when reading a release.
