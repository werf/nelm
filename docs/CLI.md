# Nelm CLI overview

```yaml
Release commands:
  release install                    Deploy a chart to Kubernetes.
  release rollback                   Rollback to a previously deployed release.
  release plan install               Plan a release install to Kubernetes.
  release uninstall                  Uninstall a Helm Release from Kubernetes.
  release list                       List all releases in a namespace.
  release history                    Show release history.
  release get                        Get information about a deployed release.

Chart commands:
  chart lint                         Lint a chart.
  chart render                       Render a chart.
  chart download                     Download a chart from a repository.
  chart upload                       Upload a chart to a repository.
  chart pack                         Pack a chart into an archive to distribute via a repository.

Secret commands:
  chart secret key create            Create a new chart secret key.
  chart secret key rotate            Reencrypt secret files with a new secret key.
  chart secret values-file edit      Interactively edit encrypted values file.
  chart secret values-file encrypt   Encrypt values file and print result to stdout.
  chart secret values-file decrypt   Decrypt values file and print result to stdout.
  chart secret file edit             Interactively edit encrypted file.
  chart secret file encrypt          Encrypt file and print result to stdout.
  chart secret file decrypt          Decrypt file and print result to stdout.

Dependency commands:
  chart dependency download          Download chart dependencies from Chart.lock.
  chart dependency update            Update Chart.lock and download chart dependencies.

Repo commands:
  repo add                           Set up a new chart repository.
  repo remove                        Remove a chart repository.
  repo update                        Update info about available charts for all chart repositories.
  repo login                         Log in to an OCI registry with charts.
  repo logout                        Log out from an OCI registry with charts.

Other commands:
  completion bash                    Generate the autocompletion script for bash
  completion fish                    Generate the autocompletion script for fish
  completion powershell              Generate the autocompletion script for powershell
  completion zsh                     Generate the autocompletion script for zsh
  version                            Show version.
```
