# Release annotations and `release get` migration

With `LegacyMetadataOnly` and `OutputNoPrint`, annotations of new-format revisions can be read without fetching the release body. The `Annotations(ctx)` getter keeps its signature.

- New revisions store annotations in Secret/ConfigMap metadata or a separate SQL table, not in `info.annotations` in the stored body. Older revisions remain readable from their bodies; there is no automatic backfill.
- Every new record includes `nelm.werf.io/released-by: "nelm"`. It is returned by get, list and history. This key is reserved; an explicit different value is rejected.
- For Secret/ConfigMap storage, annotation keys must satisfy Kubernetes rules, and all keys and values together, including the service annotation, must fit within 256 KiB. Invalid annotations are rejected before pruning history.
- Before using non-DDL SQL roles, initialize storage once as the owner of the Helm tables or a superuser. Nelm migrations are tracked in `nelm_migrations`, separate from Helm migrations. Configure annotation access explicitly as described below.
- SQL ignores stored annotations when their saved release modification time differs from the current release modification time and reads the body instead. This comparison has one-second precision.
- Writing with an older Nelm or Helm after switching to this format is unsupported: it can discard metadata annotations or leave stale SQL annotations. Annotations removed this way cannot always be recovered.
- Updating only annotations still does not trigger a deployment when the release is otherwise up to date.

## SQL role permissions

Row-level security is enabled on `nelm_release_annotations_v1`, with no default policies. Release-table access does not automatically grant annotation access. For ordinary roles, configure policies on all three tables: `releases_v1`, `custom_labels_v1` and `nelm_release_annotations_v1`.

For an existing role `nelm_user` managing namespace `app`, add the annotation policy alongside its release and label policies:

```sql
CREATE POLICY app_annotations ON nelm_release_annotations_v1 TO nelm_user
    USING (releaseNamespace = 'app')
    WITH CHECK (releaseNamespace = 'app');
```

For read-only annotation access, add `FOR SELECT` and omit `WITH CHECK`. Choose consistent namespace and operation restrictions for each table; permissions are independent. Without an annotation policy, ordinary roles cannot read stored annotations or create or update releases. Table owners, superusers and roles with `BYPASSRLS` retain PostgreSQL's usual RLS bypass.

Release and annotation writes stay atomic: if either is denied, neither change is committed. Nelm grants read access to `nelm_migrations` but does not change permissions on Helm's `gorp_migrations`. If an ordinary role cannot read that table, its owner must grant access explicitly:

```sql
GRANT SELECT ON gorp_migrations TO nelm_user;
```

With access to both migration tables, CREATE permission is not required for subsequent startup.

If a test database was initialized with an earlier draft of this migration, recreate that test database before testing the new policies; the migration id has not changed.

## Library and output changes

Replace `action.ReleaseGetResultV2` with `action.ReleaseGetResultV3`, and `.Release.StorageLabels` with `.Release.Labels`. JSON/YAML consumers must use `apiVersion: "v3"` and `release.labels` instead of `release.storageLabels`. The labels themselves are unchanged. List remains v2 and history remains v1.

The release v1/v2 `Info.Annotations` fields remain available in memory, plans and public output. Storage restores them from metadata when reading a release.
