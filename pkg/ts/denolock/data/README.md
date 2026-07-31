# Pinned Deno release

`lock.json` is embedded into the nelm binary by `../embed.go` and records the Deno release nelm runs
TypeScript charts with: the version, and per platform the target triple, the sha256 of the release
archive and the sha256 of the Deno binary inside it.

## Why the digests are committed

Deno publishes a `<archive>.sha256sum` next to every archive. Verifying a download against it is
worth nothing beyond catching a truncated transfer: it comes from the same host, over the same
connection, moments before the artifact it vouches for, so anyone able to serve a tampered archive
serves a matching checksum with it. The same goes for a digest computed on the release machine from
whatever was downloaded there and then embedded next to the blob — it only proves the blob survived
compression.

Committing the digests moves the trust anchor off the network and into this repository: the digest a
binary verifies against is one that appeared as a diff in a pull request, was reviewed, and is in git
history. It is the same reason `go.sum` and `Cargo.lock` exist. Changing what nelm executes now takes
a commit rather than control of a download.

What this does not cover: a release that was already malicious when it was pinned. Against that only
upstream provenance helps, which Deno does not publish — the v2.7.1 assets are archives and
`.sha256sum` files, nothing signed.

## Regenerating

```shell
task deno:lock                    # re-pin the version recorded in the lock
task deno:lock -- -version 2.8.0  # bump
```

It downloads every platform's archive, hashes it, unpacks it and hashes the binary, and cross-checks
each digest against the `.sha256sum` upstream publishes — a mismatch there means the download and the
checksum disagree, which is the one thing the upstream file can still tell us. Then it writes
`lock.json`. **Review the diff**: it is the whole point of the mechanism.

`task lint:deno-lock` checks the committed lock offline and runs with the other linters.

## When a digest stops matching

GitHub lets a release asset be replaced without moving the tag, so a pinned digest can stop matching
what upstream serves. A release build is what notices — it fails on the checksum, which is the
behaviour we want. To tell a replaced asset from a broken download:

```shell
task check:deno-lock-upstream
```

Five small requests, no downloads. Nothing legitimate replaces a published release asset, so a
mismatch it reports is a reason to investigate rather than to re-run `task deno:lock`.

This directory is not documentation the binary carries: `embed.go` names `lock.json` explicitly, so
this file is not embedded.
