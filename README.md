## Usage

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
  
_Standalone Nelm CLI will be provided in the future._
