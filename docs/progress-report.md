# Progress report for library consumers

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Enabling the report](#enabling-the-report)
- [Snapshots](#snapshots)
- [Report shape](#report-shape)
- [Operation types](#operation-types)
- [Statuses](#statuses)
- [Ordering and graph](#ordering-and-graph)
- [Several plans in one run](#several-plans-in-one-run)
- [Untouched resources](#untouched-resources)
- [Counting progress](#counting-progress)
- [Examples](#examples)
  - [Successful upgrade](#successful-upgrade)
  - [Failed install with a failure plan](#failed-install-with-a-failure-plan)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

Nelm can stream the progress of `ReleaseInstall` to an embedding application as a series of snapshots. Each snapshot lists every operation of the deployment plan with its current status and its dependencies, so the consumer can render progress, show what the deployment is waiting for, or draw the plan as a graph. `ReleaseUninstall` accepts the same option and behaves the same way, the text below says `ReleaseInstall` for brevity.

## Enabling the report

Pass a buffered channel in `ReleaseInstallOptions.LegacyProgressReportCh` and consume it while `ReleaseInstall` runs. `ReleaseInstall` closes the channel when it returns, so a plain `range` is all a consumer needs:

```go
reportCh := make(chan progrep.ProgressReport, 1)

var (
	last progrep.ProgressReport
	wg   sync.WaitGroup
)

wg.Add(1)
go func() {
	defer wg.Done()

	for report := range reportCh {
		last = report
	}
}()

err := action.ReleaseInstall(ctx, releaseName, releaseNamespace, action.ReleaseInstallOptions{
	Chart:                  chartPath,
	LegacyProgressReportCh: reportCh,
})

wg.Wait()
```

Rules for the channel:

- It must be buffered with capacity of at least 1, `ReleaseInstall` panics otherwise.
- Consume it concurrently with `ReleaseInstall`. Intermediate snapshots are sent without blocking and are dropped when the channel is full, but the final snapshot is sent with a blocking send, so a consumer that stops reading before `ReleaseInstall` returns deadlocks the deployment.
- Do not close the channel yourself: `ReleaseInstall` closes it when it returns, on every path, including early errors. Snapshots still in the buffer at that moment are delivered before the `range` ends. After a `Timeout` the deployment may still be winding down inside nelm, its snapshots after the return are dropped.
- A snapshot handed to the consumer is never modified afterwards.
- `AutoRollback` cannot be combined with `LegacyProgressReportCh`: `ReleaseInstall` returns an error before doing anything. Rollback reporting needs its own design.

## Snapshots

1. The first snapshot arrives when the plan is built, before any operation runs: every operation is listed, all `Pending`, untouched resources already `Completed`.
2. A snapshot arrives on every status change, unless the consumer has not taken the previous one yet, then it is skipped.
3. The final snapshot arrives when the deployment is over, with a blocking send, so it is never skipped. Nothing in it is `Pending`: what did not run is `Canceled`.

Every snapshot is complete and self-contained: it lists every operation reported so far with its current status, so keeping the latest one is enough, and a skipped snapshot loses nothing but intermediate statuses. The set of operations is fixed from the first snapshot, with one exception: after a failure the operations of the failure plan are appended, so the report grows once, see [Several plans in one run](#several-plans-in-one-run).

## Report shape

The types live in `github.com/werf/nelm/pkg/legacy/progrep`. A report has a single field, `Operations`, a list of operations with the following fields:

| Field | Meaning |
|---|---|
| `ID` | Unique within the report. Opaque key that `DependsOn` refers to, do not parse it: the other fields carry everything it encodes. Operations of the second and every following plan of a run are prefixed with the plan's ordinal number, `2/...`, see [Several plans in one run](#several-plans-in-one-run). |
| `Type` | What the operation does, see [Operation types](#operation-types). |
| `Category` | `meta`, `resource`, `track` or `release`, see [Operation types](#operation-types). |
| `Status` | `Pending`, `Progressing`, `Completed`, `Failed` or `Canceled`, see [Statuses](#statuses). |
| `Iteration` | Distinguishes several operations of the same type on the same resource within one plan, e.g. a resource deployed twice by a chart. Usually 0. |
| `GroupVersionKind` | Group, version and kind of the Kubernetes object. |
| `Name` | Name of the Kubernetes object. |
| `Namespace` | Effective namespace of the Kubernetes object: the release namespace for namespaced objects without an explicit one, empty for cluster-scoped objects. |
| `DependsOn` | IDs of the operations that must finish before this one starts. Always present, empty for operations without predecessors. |

`GroupVersionKind`, `Name` and `Namespace` describe the Kubernetes object of `resource` and `track` operations. `meta` and `release` operations have no object, these fields are empty for them.

## Operation types

| Category | Types | Notes |
|---|---|---|
| `meta` | `StageStart`, `StageEnd` | Boundaries of deployment stages (`init`, `pre-install`, `install`, ...) and of `werf.io/weight` groups inside them. They do nothing themselves, but they carry the structure: operations of a later stage depend on operations of an earlier one through them. |
| `resource` | `Create`, `Update`, `Apply`, `Recreate`, `Delete`, `NoOp` | Mutate a Kubernetes object. `NoOp` is reserved for [untouched resources](#untouched-resources). |
| `track` | `TrackReadiness`, `TrackPresence`, `TrackAbsence` | Wait for an object to become ready, appear or disappear. Never mutate anything. |
| `release` | `CreateRelease`, `UpdateRelease`, `DeleteRelease` | Mutate the Helm release record. `CreateRelease` is among the first operations, `UpdateRelease` marks the release deployed at the very end, or failed in a failure plan. |

## Statuses

| Status | Meaning |
|---|---|
| `Pending` | Not started yet. Every operation of a plan starts with it. |
| `Progressing` | Running. |
| `Completed` | Finished successfully. For `NoOp` it means nothing was done, see [Untouched resources](#untouched-resources). |
| `Failed` | Finished with an error. Nelm stops scheduling new operations of the plan after that. |
| `Canceled` | Never started because its plan stopped before reaching it, e.g. after another operation failed. The final snapshot never contains `Pending`: everything that did not run is `Canceled`. |

## Ordering and graph

Nelm deploys a release by building a directed acyclic graph of operations and executing it: an operation starts once all of its predecessors are done, operations that do not depend on each other run in parallel. The report exposes that graph as is, `DependsOn` are the edges.

Operations are listed in execution order, and the order is deterministic: it never changes between snapshots, only statuses do, so consecutive snapshots diff cleanly.

`DependsOn` reproduces the whole plan graph, including edges to and from `meta` operations. A plan has a single root, the `StageStart` of its first stage, and a single sink, the `StageEnd` of its last stage. Filtering `meta` operations out of the graph breaks connectivity between stages, filter them for display only.

## Several plans in one run

After a failure the report grows: the operations of the failure plan are appended after the install plan. The failure plan marks the release failed and deletes the resources that ask for it with `werf.io/delete-policy: failed`. What the consumer sees:

- New operations with an ordinal prefix in `ID`: `2/...`, and `3/...` for a further plan. The first plan is unprefixed, so a run without failures never shows prefixes. Both `ID` and `DependsOn` use the prefixed form.
- Operations of the install plan that never ran turn `Canceled`.
- The graph stays connected: the root operations of the new plan depend on the sink operations of the plan before it, so the final snapshot reads top to bottom as a chronology of the whole run.

## Untouched resources

An untouched resource is a chart resource the plan has no operations for: nothing needs to be done, or a resource policy forbids doing it. They are reported so that progress counts cover the whole chart: ten unchanged resources and two updated ones show as 12 operations, not 2.

- Type `NoOp`, category `resource`, status `Completed` from the first snapshot.
- `DependsOn` is empty and no operation depends on them: they are not part of the execution flow.
- They come first in the slice, before the plan operations.
- Only the first plan reports them. A failure plan acts upon a few resources of a release the first plan has already described in full.
- `NoOp`/`Completed` states that nothing was done to the resource during the release. The resource may be absent from the cluster (creation skipped by `werf.io/resource-policy`) or differ from the chart (update skipped by the same policy). It is not a statement about the resource being present or in the desired state.

When the release is skipped because the cluster already matches the chart, the report consists of untouched resources alone.

## Counting progress

Count `resource` and `track` operations only. `meta` operations are stage boundaries and `release` operations are bookkeeping of the release record, both inflate the totals without telling the user anything about their resources:

```go
var completed, remaining int
for _, op := range report.Operations {
	if op.Category != progrep.OperationCategoryResource && op.Category != progrep.OperationCategoryTrack {
		continue
	}

	if op.Status == progrep.OperationStatusCompleted {
		completed++
	} else {
		remaining++
	}
}
```

## Examples

The examples show final snapshots as serialized, with the JSON keys.

### Successful upgrade

A final snapshot of an upgrade where one ConfigMap is unchanged and two others are updated, abridged. The unchanged ConfigMap is an untouched resource and comes first. The two updated ConfigMaps belong to the same stage and have no dependencies on each other, so both `Apply` operations depend on the same `StageStart` and run in parallel, and the `StageEnd` waits for both trackings.

```yaml
operations:
- id: noop/1/0/my-namespace::ConfigMap:cm-unchanged
  type: NoOp
  category: resource
  status: Completed
  Group: ""
  Version: v1
  Kind: ConfigMap
  name: cm-unchanged
  namespace: my-namespace
  dependsOn: []
- id: noop/1/0/stage/init/start
  type: StageStart
  category: meta
  status: Completed
  dependsOn: []
- id: create-release/1/0/my-namespace:my-release:2
  type: CreateRelease
  category: release
  status: Completed
  dependsOn: [noop/1/0/stage/init/start]
- id: noop/1/0/stage/init/end
  type: StageEnd
  category: meta
  status: Completed
  dependsOn: [create-release/1/0/my-namespace:my-release:2]
- id: noop/1/0/stage/install/start
  type: StageStart
  category: meta
  status: Completed
  dependsOn: [noop/1/0/stage/init/end]
- id: apply/1/0/::ConfigMap:cm-changed
  type: Apply
  category: resource
  status: Completed
  Group: ""
  Version: v1
  Kind: ConfigMap
  name: cm-changed
  namespace: my-namespace
  dependsOn: [noop/1/0/stage/install/start]
- id: apply/1/0/::ConfigMap:cm-other
  type: Apply
  category: resource
  status: Completed
  Group: ""
  Version: v1
  Kind: ConfigMap
  name: cm-other
  namespace: my-namespace
  dependsOn: [noop/1/0/stage/install/start]
- id: track-readiness/1/0/::ConfigMap:cm-changed
  type: TrackReadiness
  category: track
  status: Completed
  Group: ""
  Version: v1
  Kind: ConfigMap
  name: cm-changed
  namespace: my-namespace
  dependsOn: [apply/1/0/::ConfigMap:cm-changed]
- id: track-readiness/1/0/::ConfigMap:cm-other
  type: TrackReadiness
  category: track
  status: Completed
  Group: ""
  Version: v1
  Kind: ConfigMap
  name: cm-other
  namespace: my-namespace
  dependsOn: [apply/1/0/::ConfigMap:cm-other]
- id: noop/1/0/stage/install/end
  type: StageEnd
  category: meta
  status: Completed
  dependsOn:
  - track-readiness/1/0/::ConfigMap:cm-changed
  - track-readiness/1/0/::ConfigMap:cm-other
- id: noop/1/0/stage/final/start
  type: StageStart
  category: meta
  status: Completed
  dependsOn: [noop/1/0/stage/install/end]
- id: update-release/1/0/my-namespace:my-release:2
  type: UpdateRelease
  category: release
  status: Completed
  dependsOn: [noop/1/0/stage/final/start]
- id: noop/1/0/stage/final/end
  type: StageEnd
  category: meta
  status: Completed
  dependsOn: [update-release/1/0/my-namespace:my-release:2]
```

Counting `resource` and `track` operations gives 5 completed and 0 remaining.

### Failed install with a failure plan

A final snapshot of a failed install, abridged. The chart has a ConfigMap in weight -10, a Job in weight 0 that fails and carries `werf.io/delete-policy: failed`, and a ConfigMap in weight 10 that is never created. The failure plan marks the release failed and deletes the Job.

```yaml
operations:
- id: noop/1/0/stage/init/start
  type: StageStart
  category: meta
  status: Completed
  dependsOn: []
- id: create-release/1/0/my-namespace:my-release:1
  type: CreateRelease
  category: release
  status: Completed
  dependsOn: [noop/1/0/stage/init/start]
- id: noop/1/0/stage/init/end
  type: StageEnd
  category: meta
  status: Completed
  dependsOn: [create-release/1/0/my-namespace:my-release:1]
# ... stage install, weight -10: cm-before created and tracked ...
- id: noop/1/0/stage/install/weight:0/start
  type: StageStart
  category: meta
  status: Completed
  dependsOn: [noop/1/0/stage/install/weight:-10/end]
- id: create/1/0/:batch:Job:failing-job
  type: Create
  category: resource
  status: Completed
  Group: batch
  Version: v1
  Kind: Job
  name: failing-job
  namespace: my-namespace
  dependsOn: [noop/1/0/stage/install/weight:0/start]
- id: track-readiness/1/0/:batch:Job:failing-job
  type: TrackReadiness
  category: track
  status: Failed
  Group: batch
  Version: v1
  Kind: Job
  name: failing-job
  namespace: my-namespace
  dependsOn: [create/1/0/:batch:Job:failing-job]
- id: noop/1/0/stage/install/weight:0/end
  type: StageEnd
  category: meta
  status: Canceled
  dependsOn: [track-readiness/1/0/:batch:Job:failing-job]
# ... weight 10: cm-after and its tracking, stage final and update-release, all Canceled ...
- id: noop/1/0/stage/final/end
  type: StageEnd
  category: meta
  status: Canceled
  dependsOn: [update-release/1/0/my-namespace:my-release:1]
- id: 2/noop/1/0/stage/init/start
  type: StageStart
  category: meta
  status: Completed
  dependsOn: [noop/1/0/stage/final/end]
- id: 2/update-release/1/0/my-namespace:my-release:1
  type: UpdateRelease
  category: release
  status: Completed
  dependsOn: [2/noop/1/0/stage/init/start]
# ... 2/noop/1/0/stage/init/end, 2/noop/1/0/stage/uninstall/start ...
- id: 2/delete/1/0/:batch:Job:failing-job
  type: Delete
  category: resource
  status: Completed
  Group: batch
  Version: v1
  Kind: Job
  name: failing-job
  namespace: my-namespace
  dependsOn: [2/noop/1/0/stage/uninstall/start]
- id: 2/track-absence/1/0/:batch:Job:failing-job
  type: TrackAbsence
  category: track
  status: Completed
  Group: batch
  Version: v1
  Kind: Job
  name: failing-job
  namespace: my-namespace
  dependsOn: [2/delete/1/0/:batch:Job:failing-job]
- id: 2/noop/1/0/stage/uninstall/end
  type: StageEnd
  category: meta
  status: Completed
  dependsOn: [2/track-absence/1/0/:batch:Job:failing-job]
```

Counting `resource` and `track` operations gives 5 completed and 3 remaining: the failed tracking of the Job and the canceled ConfigMap with its tracking.
