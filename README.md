**Nelm** is the new deployment engine for [werf](https://github.com/werf/werf/).

**Nelm** is meant to be a direct Helm 3 replacement, providing first-class Helm-chart support, yet improving on what Helm 3 offers.

The deployment subsystem of Helm 3 was rewritten from scratch to fix numerous Helm issues like [this one](https://github.com/helm/helm/issues/7219) and to introduce major improvements such as "terraform plan"-like functionality, replacement of 3-Way Merge with Server-Side Apply, advanced resource tracking, advanced resource ordering and resource lifecycle capabilities.

_At the moment Nelm does not have its own command-line interface, but we will provide it later. To use Nelm now, you can use werf for deployment of Helm charts since werf uses Nelm under the hood._

## Getting started

1. Download [werf](https://github.com/werf/werf/releases/latest).
2. Initialize the demo project:
```bash
git clone https://github.com/werf/nelm
cp -r nelm/examples/basic /tmp/example
cd /tmp/example
git init
git add .
git commit -m init
```
3. Enable Nelm in werf:
```bash
export WERF_NELM=1
```
4. Check what's going to change in the Kubernetes cluster on next release:
```bash
werf plan --env dev --dev
```
5. Deploy new release:
```bash
werf converge --env dev --dev
```
