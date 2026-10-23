# Issue draft for werf/nelm (copy into GitHub)

**Title:** Upgrade deadlocks when purged PVCs are deleted before Deployment update (`pre-pre-uninstall` + `TrackAbsence`)

## Summary

On `nelm release install` / upgrade, resources removed from the chart (purged from the previous release) are scheduled in stage `pre-pre-uninstall`, **before** `install`. For each deleted object Nelm always runs `Delete` and then `TrackAbsence` (`MustTrackAbsence: true`).

That ordering differs from Helm 3 and causes a hard deadlock for `PersistentVolumeClaim`s protected by `kubernetes.io/pvc-protection` while a Pod from the **old** Deployment still mounts them.

## Helm 3 behavior (works)

In Helm 3 `kube.Client.Update`:

1. Create/update resources from the new release (including a new PVC and Deployment roll).
2. Delete objects that are no longer in the chart.
3. `--wait` waits for **readiness** of the new release resources, not for every purged PVC to become fully absent.

Workloads move off the old claim first; then the PVC can finish deletion.

## Nelm behavior (deadlocks)

Relevant code (before fix):

- `pkg/plan/resource_info.go` → `buildDeletableResourceInfo`: for non-uninstall deploy types, `stage = StagePrePreUninstall`.
- `pkg/common/common.go` → `StagesOrdered`: `pre-pre-uninstall` runs **before** `install`.
- `pkg/plan/plan_build.go` → `addDeleteResourcesOps`: `Delete` chained with `TrackAbsence`.
- `MustTrackAbsence` is always `true` when `MustDelete` is true.

Observed progress during a hang:

```text
RESOURCE (→ABSENT)   PersistentVolumeClaim/...  WAITING
RESOURCE (→ABSENT)   PersistentVolume/...       WAITING
Secret/...           ABSENT
StorageClass/...     ABSENT
```

Deployment/pods stay on the old PVC; install stage never starts. Cancel + retry often “works” because the pending revision / orphaned inventory changes and the second plan no longer blocks on the same deletes before install.

## Reproduction (minimal)

1. Release with a Deployment mounting a PVC (static SMB PV + `Retain` is a strong case).
2. Upgrade so the PVC name changes (e.g. volume id suffix changes) → plan: create new PVC/PV, update Deployment claimName, delete old PVC/PV.
3. Run `nelm release install` with readiness tracking enabled.
4. Observe hang on `→ABSENT` for the old PVC/PV while Deployment is still unchanged.

## Expected

Closer to Helm: **install/update first**, then purge previous-release resources (and only then wait for absence, if at all). After the Deployment rolls to the new claim, `pvc-protection` can clear and the old PVC can disappear.

## Proposed fix

Schedule purged deletable resources on **`StageUninstall`** (after `StageInstall`) for upgrade/install/rollback as well as uninstall, instead of `StagePrePreUninstall`.

PoC branch: `fix/purge-resources-after-install` (fork).

Optional follow-ups:

- Make `MustTrackAbsence` configurable / skip for PV+PVC with `Retain`.
- Document the stage order vs Helm for resource replacement.

## Environment

- Nelm 1.29.x / 1.30.x
- Charts that replace PVC identity across upgrades (e.g. generated volume id)
- CSI / SMB volumes with PVC protection finalizers

## Related

- https://github.com/werf/nelm/issues/99 (hang involving PVC on dismiss)
- `werf.io/track-termination-mode: NonBlocking` does **not** help: it only affects readiness tracking, not `TrackAbsence`
