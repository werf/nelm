**Nelm** is meant to be a direct **Helm 3** replacement, providing first-class **Helm-chart** support, yet improving on what **Helm 3** offers.

**Nelm** is used as the deployment engine in [werf](https://github.com/werf/werf/). Standalone Nelm CLI is also available, although it is in active development for now and not recommended for general use.

**Nelm** is based on **Helm 3** — some parts of it improved and some, like the deployment subsystem, are rewritten from scratch to introduce:
* `terraform plan`-like capabilities
* Replacement of 3-Way Merge with Server-Side Apply
* Improved resource tracking built from the ground up
* Advanced resource ordering capabilities
* Flexible resource lifecycle management
* Fixes for numerous issues like [this one](https://github.com/helm/helm/issues/7219) and other improvements

## Getting started with Nelm (experimental)

> Standalone Nelm is in active development and *is not recommended* for production use. Nelm commands, options and their defaults are subject to change.

1. Build and install Nelm:
```bash
git clone https://github.com/werf/nelm
cd nelm
go build ./cmd/nelm/
sudo install -t /usr/local/bin nelm
```
2. Create new chart:
```bash
mkdir ../mychart
cd ../mychart

cat > Chart.yaml <<EOF
apiVersion: v2
name: mychart
version: 1.0.0
dependencies:
- name: cert-manager
  version: 1.13.3
  repository: https://charts.jetstack.io
EOF

cat > values.yaml <<EOF
cert-manager:
  installCRDs: true
  startupapicheck:
    enabled: false
EOF

nelm chart dependency build
```
3. Check what's going to happen on next release:
```bash
nelm plan deploy -n myproject -r myproject
```
4. Deploy new release:
```bash
nelm release deploy -n myproject -r myproject
```

## Getting started with Nelm via werf

1. Download [werf v2](https://github.com/werf/werf/releases)
2. Create new project:
```bash
git init myproject
cd myproject
mkdir .helm

cat > .helm/Chart.yaml <<EOF
dependencies:
- name: cert-manager
  version: 1.13.3
  repository: https://charts.jetstack.io
EOF

cat > .helm/values.yaml <<EOF
cert-manager:
  installCRDs: true
  startupapicheck:
    enabled: false
EOF

cat > werf.yaml <<EOF
configVersion: 1
project: myproject
EOF

cat > .gitignore <<EOF
/.helm/charts/*.tgz
EOF

werf helm dependency update .helm
git add .
git commit -m init
```
3. Check what's going to happen on next release:
```bash
werf plan --env dev
```
4. Deploy new release:
```bash
werf converge --env dev
```
5. Proceed to [werf documentation](https://werf.io/docs/v2/usage/deploy/overview.html) to learn more.

