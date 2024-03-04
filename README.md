**Nelm** is meant to be a direct **Helm 3** replacement, providing first-class **Helm-chart** support, yet improving on what **Helm 3** offers.

**Nelm** is used as the deployment engine in [werf](https://github.com/werf/werf/), but **Nelm** will have its own CLI in the future for those who don't need everything that werf offers.

**Nelm** is based on **Helm 3**: some parts of it improved and some, like the deployment subsystem, are rewritten from scratch to fix numerous issues like [this one](https://github.com/helm/helm/issues/7219) and to introduce major improvements such as:
* `terraform plan`-like capabilities
* Replacement of 3-Way Merge with Server-Side Apply
* Improved resource tracking built from the ground up
* Advanced resource ordering capabilities
* Flexible resource lifecycle management and more

## Getting started with Nelm via werf

1. Download [werf](https://github.com/werf/werf/releases/latest)
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
3. _(for werf 1.2.295 or older)_ Enable Nelm in werf:
```bash
export WERF_NELM=1
```
4. Check what's going to happen on next release:
```bash
werf plan --env dev
```
5. Deploy new release:
```bash
werf converge --env dev
```
6. Proceed to [werf documentation](https://werf.io/documentation/v1.2/usage/deploy/overview.html) to learn more.

## Getting started with standalone Nelm

_Not implemented yet_
