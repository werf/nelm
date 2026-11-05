# Pinned Node.js release

`lock.json` is embedded into the nelm binary by `../lock.go` and records the Node.js release nelm runs
TypeScript charts with: the version, the version of npm the release archives ship, and per platform
the archive target, the sha256 of the release archive, the sha256 of the Node.js binary inside it and
the tree digest of the npm package inside it.

The npm digest (`TreeSHA256` in `../lock.go`) is the sha256 of the sorted `<sha256 of file>  <path>`
lines of every regular file in the package, so it does not depend on whether npm came from the
release archive or from the blob embedded into nelm, nor on file modes or timestamps.

## Why the digests are committed

Node.js publishes `SHASUMS256.txt` next to the archives. Verifying a download against it, without
checking its GPG signature, is worth nothing beyond catching a truncated transfer: it comes from the
same host, over the same connection, moments before the artifact it vouches for, so anyone able to
serve a tampered archive serves a matching checksum with it. The same goes for a digest computed on
the release machine from whatever was downloaded there and then embedded next to the blob — it only
proves the blob survived compression.

Committing the digests moves the trust anchor off the network and into this repository: the digest a
binary verifies against is one that appeared as a diff in a pull request, was reviewed, and is in git
history. It is the same reason `go.sum` and `Cargo.lock` exist. Changing what nelm executes now takes
a commit rather than control of a download.

What this does not cover: a release that was already malicious when it was pinned. The GPG signature
of `SHASUMS256.txt` by the Node.js releasers would help there, and is not checked yet.

## Regenerating

```shell
task node:lock                       # re-pin the version recorded in the lock
task node:lock -- -version 26.14.0   # bump
```

It downloads every platform's archive, hashes it, cross-checks the digest against `SHASUMS256.txt` —
a mismatch there means the download and the checksum disagree, which is the one thing the upstream
file can still tell us — then hashes the Node.js binary and the npm package inside it. Then it writes
`lock.json`. **Review the diff**: it is the whole point of the mechanism.

`task lint:node-lock` checks the committed lock offline and runs with the other linters.

## When a digest stops matching

To tell a replaced release asset from a broken download:

```shell
task check:node-lock-upstream
```

Two small requests, no downloads. Nothing legitimate replaces a published release asset, so a mismatch
it reports is a reason to investigate rather than to re-run `task node:lock`.

This directory is not documentation the binary carries: `lock.go` names `lock.json` explicitly, so
this file is not embedded.
