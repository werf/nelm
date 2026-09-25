# Embedded JSON schemas

These archives are embedded into the nelm binary by `../embed.go` and hold the JSON schemas nelm
validates resources against without network access:

- `kubernetes.tar.gz` — the Kubernetes API schemas of several minor versions merged into one flat
  set. The newest is the `kubeconformValidationSchemasUpstreamKubeVersion` variable of the Taskfile
  exactly as pinned, since that is the version nelm validates against; the minors right before it are
  taken at the latest patch upstream has, which is decided from the upstream listing at generation
  time rather than computed. The newest version contributes all of its schemas, and every older one
  only adds the schemas that no newer version has any more, so resources of API versions removed
  along the way stay validatable while everything else is validated against its newest schema.
- `crds.tar.gz` — a catalog of CRD schemas.
- `index.json` — what each archive contains, and its digest.

They are committed on purpose. `go:embed` resolves at compile time from the files present in the
module as distributed, and the module cache is read-only, so generating them at build time would
leave anyone importing nelm as a library, or installing it with `go install`, without schemas. It
also keeps builds offline and makes a nelm commit fully determine the binary it produces.

## Regenerating

```shell
task generate:validation-schemas          # after bumping the pinned Kubernetes version
task generate:validation-schemas:force    # to pull whatever upstream has now
```

The plain task only regenerates what the committed archives do not already cover, and it decides that
without any network access, by comparing the index against the repository, ref, pinned version and
number of minors it was asked for. It therefore cannot notice that a ref has moved or that a new patch
was released: refreshing the CRD catalog, or picking up schema fixes for an unchanged Kubernetes
version, needs the `:force` variant.

`task lint:validation-schemas` is offline for the same reason, so it takes the collected patches from
the index as recorded and only checks the pin against the Taskfile: the newest version and the window
of minors around it.

That the network stays out of it is deliberate — builds are offline and a nelm commit determines the
binary it produces — so staleness is reported separately instead:

```shell
task check:validation-schemas-upstream
```

Two requests, no downloads, and it runs daily in CI. For the CRD catalog it compares the commit, since
the whole repository is taken. For the Kubernetes schemas it lists the repository and fails when a
collected older minor has a newer patch upstream, or when the git tree of a collected version changed.
The commit is deliberately not compared there: that repository's ref moves whenever any version is
touched, while the directories we take are frozen snapshots, so it would cry stale on every unrelated
release. A newer patch of the pinned version is only reported, since bumping the pin is a decision.

Commit the changed archives along with `index.json`. `task lint:validation-schemas` checks that the
two agree, and runs offline.

Both tasks also fail when the archives add up to more than `-max-bundles-size`, 5 MiB by default:
every byte of them is carried by each nelm binary and by each module importing nelm, so the size is a
budget rather than whatever upstream happens to produce. Collecting fewer Kubernetes minor versions is
the usual way back under it; raising the flag is the other, when the budget is meant to grow.

This directory is not documentation the binary carries: `embed.go` names the archives explicitly, so
this file is not embedded.
