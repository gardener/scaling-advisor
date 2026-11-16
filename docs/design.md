# Scaling Advisor Design 

## High Level View

The `scaling-advisor` is an *advisory controller* that operates across the control cluster, the target custer and the catalog cluster.

- **Control cluster**: hosts the custom resources that provide constraints, configuration, advice and feedback for scaling the target cluster.
- **Catalog cluster**: the cluster in which common master data - catalog objects are stored. Generally a global cluster meant to hold read-only project data. Read only access.
- **Target cluster**: the workload cluster for whom scaling advice should be generated. Catalog objects may be defined or overriden here.

The operator watches clusters and runs a reconcile loop on every relevant change:

1. Reads [ScalingConstraint](../api/core/v1alpha1/scalingconstraints.go#L26) (node pools, strategies), [ScalingFeedback](../api/core/v1alpha1/scalingfeedback.go#L29) from the control cluster.

1. Read [NodeTemplateSet](../api/core/v1alpha1/nodetemplateset.go#L15) (available node templates), [InstanceTypeDefSet](../api/core/v1alpha1/instancetypedefset.go#L19) (instance capacities), and [InstancePriceSet](../api/core/v1alpha1/instancepriceset.go#L18) (pricing) from the catalog cluster.
1. Read custom overrides for the `NodeTemplateSet`, `InstanceTypeDefSet` and `InstancePriceSet` from the target cluster
2. Reads scheduling-relevant objects (Nodes, Pods, PVs, PVCs, StorageClasses, PriorityClasses, RuntimeClasses, CSINodes) from the target cluster.
3. Builds a [Request](../planner/api/types.go#L34) and invokes the [ScalingPlanner](../planner/api/types.go#L280), which delivers one or more [Response](../planner/api/types.go#L74)s on a result channel.
4. Converts each `Response` into a [ScalingAdvice](../api/core/v1alpha1/scalingadvice.go#L30) resource written to the control cluster. `ScalingAdvice.Spec` carries a [ScaleOutPlan](../api/core/v1alpha1/scalingadvice.go#L79) and/or [ScaleInPlan](../api/core/v1alpha1/scalingadvice.go#L90).

A **Lifecycle Manager** (e.g. [machine-controller-manager](https://github.com/gardener/machine-controller-manager)) runs in the control cluster. It:

- Watches `ScalingAdvice` and provisions or decommissions machine instances accordingly.
- Writes a [ScalingFeedback](../api/core/v1alpha1/scalingfeedback.go#L29) resource (owned by its `ScalingAdvice`, sharing the same name/namespace) with `ScaleOutFeedback`/`ScaleInFeedback` and sets [AcknowledgedAt](../api/core/v1alpha1/scalingfeedback.go#L53) when it has acted on the advice.

The `scaling-advisor` reads `ScalingFeedback` to pace subsequent advice: it will not produce a new `ScalingAdvice` until the lifecycle manager has set `ScalingFeedback.Spec.AcknowledgedAt`, or the current `ScalingAdvice` is determined to be outdated and marked `Expired`.

![High Level Operational View](./assets/high-level.svg)


## Custom Resources and Types

The following diagram illustrates the custom resources and their associated types within the `/api` module.

### Scaling Constraints and Catalog Resources

![Scaling Constraints and Catalog Resources](./assets/types-crds-constraints.svg)

| Type | Description |
|---|---|
| [`ScalingConstraint`](../api/core/v1alpha1/scalingconstraints.go#L26) | CRD (control cluster): defines node pools and strategies used to generate `ScalingAdvice`. |
| [`ScalingConstraintSpec`](../api/core/v1alpha1/scalingconstraints.go#L49) | Planner and simulator strategy selectors plus the list of `NodePool`s. |
| [`ScalingConstraintStatus`](../api/core/v1alpha1/scalingconstraints.go#L82) | Validation conditions set by the operator after reconciling a `ScalingConstraint`. |
| [`NodePool`](../api/core/v1alpha1/scalingconstraints.go#L93) | A scalable node pool: region, zones, template references, priority, and optional resource limits. |
| [`NodeTemplateRef`](../api/core/v1alpha1/scalingconstraints.go#L120) | References a `NodeTemplate` by name with a relative priority within its parent `NodePool`. |
| [`ResourceLimits`](../api/core/v1alpha1/scalingconstraints.go#L134) | Min/max aggregate resource bounds for the node pool. |
| [`ScalingParams`](../api/core/v1alpha1/scalingconstraints.go#L145) | Per-pool scale-in tuning: utilization thresholds and the minimum underutilized duration. |
| [`ResourceUtilizationThreshold`](../api/core/v1alpha1/scalingconstraints.go#L153) | Pairs a resource name (e.g. `cpu`, `memory`) with its utilization threshold fraction. |
| [`PlannerStrategy`](../api/common/types.go#L49) | Enumeration: high-level ordering policy (scale-in first, scale-out first, or consolidated). |
| [`SimulatorStrategy`](../api/common/types.go#L92) | Enumeration: `SingleNodeMultiSim` (one node per simulation) vs `MultiNodeSingleSim` (all nodes in one). |
| [`NodeScoringStrategy`](../api/common/types.go#L149) | Enumeration: algorithm for ranking scale-out candidates — `LeastWaste` or `LeastCost`. |
| [`NodeTemplateSet`](../api/core/v1alpha1/nodetemplateset.go#L15) | CRD (catalog cluster): holds all `NodeTemplate`s available to the scaling-advisor. |
| [`NodeTemplate`](../api/core/v1alpha1/nodetemplateset.go#L49) | Node configuration for one instance type: reserved/extended resources and the instance type name. |
| [`InstanceTypeDefSet`](../api/core/v1alpha1/instancetypedefset.go#L19) | CRD (catalog cluster): holds all `InstanceTypeDef`s for a given cloud provider. |
| [`InstanceTypeDef`](../api/core/v1alpha1/instancetypedefset.go#L53) | Base hardware specification for a cloud instance type: CPU architecture and raw resource capacity. |
| [`InstancePriceSet`](../api/core/v1alpha1/instancepriceset.go#L18) | CRD (catalog cluster): holds pricing data for all instance types of a given cloud provider. |
| [`InstancePrice`](../api/core/v1alpha1/instancepriceset.go#L76) | Price for one `(instanceType, region, capacityType)` combination, with optional per-resource unit prices. |
| [`ResourceUnitPrice`](../api/core/v1alpha1/instancepriceset.go#L83) | Unit price for a single resource (e.g. GPU hours) within an `InstancePrice`. |
| [`CloudProvider`](../api/common/types.go#L167) | Enumeration: cloud provider identity — `aws`, `gcp`, `azure`, `ali`, `openstack`. |
| [`CapacityType`](../api/common/types.go#L219) | Enumeration: compute capacity procurement model — `on-demand` or `spot`. |

### Scaling Advice and Scaling Feedback

![Scaling Advice and Scaling Feedback](./assets/types-crds-advice.svg)

| Type | Description |
|---|---|
| [`ScalingAdvice`](../api/core/v1alpha1/scalingadvice.go#L30) | CRD (control cluster): scaling advice produced by the operator, consumed by the lifecycle manager. |
| [`ScalingAdviceSpec`](../api/core/v1alpha1/scalingadvice.go#L53) | The generated plan: optional scale-out, scale-in, and surge plans plus a reference to the originating `ScalingConstraint`. |
| [`ScalingAdviceStatus`](../api/core/v1alpha1/scalingadvice.go#L71) | Operator-set conditions on the `ScalingAdvice` (e.g. `Expired`). |
| [`ScaleOutPlan`](../api/core/v1alpha1/scalingadvice.go#L80) | List of scale-out items plus any pod names that could not be satisfied. |
| [`ScaleInPlan`](../api/core/v1alpha1/scalingadvice.go#L91) | List of specific nodes recommended for removal. |
| [`ScaleOutItem`](../api/core/v1alpha1/scalingadvice.go#L109) | Recommendation to add `delta` nodes at a given `NodePlacement`, with the current replica count and priority. |
| [`ScaleInItem`](../api/core/v1alpha1/scalingadvice.go#L99) | Recommendation to remove a specific node identified by name, placement, and priority. |
| [`NodePlacement`](../api/core/v1alpha1/scalingadvice.go#L132) | Identifies the target pool, template, instance type, region, and zone for a scaling operation. |
| [`ScalingAdviceDiagnostic`](../api/core/v1alpha1/scalingadvice.go#L173) | References to trace and run-results logs for debugging a `ScalingAdvice`. |
| [`ScalingFeedback`](../api/core/v1alpha1/scalingfeedback.go#L29) | CRD (control cluster): written by the lifecycle manager in response to a `ScalingAdvice`; shares its name/namespace and is owned by it. |
| [`ScalingFeedbackSpec`](../api/core/v1alpha1/scalingfeedback.go#L47) | Lifecycle manager response: acknowledgement timestamp and per-item scale-out/scale-in feedback. |
| [`ScaleOutFeedback`](../api/core/v1alpha1/scalingfeedback.go#L62) | Feedback on the scale-out plan: one `ScaleOutItemFeedback` per `ScaleOutItem`. |
| [`ScaleOutItemFeedback`](../api/core/v1alpha1/scalingfeedback.go#L71) | Per-item feedback: fail count, optional backoff deadline, and a human-readable message. |
| [`ScaleInFeedback`](../api/core/v1alpha1/scalingfeedback.go#L90) | Feedback on the scale-in plan: names of nodes accepted for removal by the lifecycle manager. |

## Modules
The scaling-advisor has been structured into several Go modules, some of which are independently reusable.

![Modules](./assets/modules.svg)

| Module Path | Description |
|---|---|
| `/api` | Base CRD types and common API constants. Foundation for all other modules. |
| `/minkapi/api` | Interface definitions for the minimal in-memory Kubernetes API (MinKAPI). |
| `/simulation/api` | Interface and type definitions for the simulation subsystem. |
| `/planner/api` | Interface and type definitions for the planner subsystem, including `ScalingPlanner`, `Request`, and `Response`. |
| `/planner/api/helper` | Adapter and query helpers that reference both planner and simulation API types (`AsNodeInfo`, `AsPodInfo`, `GetUnscheduledPods`, etc.). Follows the [k8s core/v1/helper] convention: helpers too dependency-heavy for the API package itself live in a sub-package so that lightweight consumers (e.g. `minkapi`) need not pull in `simulation/api`. |
| `/common` | Shared utilities: node, pod, object, and I/O helpers. Depends only on `api` and `minkapi/api`; higher-level helpers (planner/simulation-aware) live in `planner/api/helper`. |
| `/minkapi` | In-memory Kubernetes API implementation with sandboxed views, used by the scheduler during simulation. |
| `/simulation` | Simulation engine: runs the embedded kube-scheduler against a sandboxed MinKAPI view to produce `SimulationResult`s. |
| `/simulation/simvolume` | Simulated PVC/PV binding operations. Kept separate from `common/volutil` because it is simulation specific and requires `planner/api` and `simulation/api` types. |
| `/samples` | Test fixtures and scenario generators (pods, PVCs, PVs, storage classes). Used by `simulation` and `planner` tests. |
| `/planner` | Core planning logic: `ScaleInSimulator`, [ScaleOutSimulator](#scaleoutsimulator), [`ScaleInCandidateSelector`](#scaleincandidateselector), and the default [ScalingPlanner](#scalingplanner) implementations. |
| `/service` | HTTP/gRPC service wrapper (currently a stub) that exposes the planner as a local network endpoint for testing/debugging. |
| `/operator` | `scaling-advisor` operator binary *(currently a stub - will wire up planner, minkapi, and service)*. |
| `/tools` | CLI utilities (`scadctl`) for generating instance type definitions and pricing data. |

[k8s core/v1/helper]: https://pkg.go.dev/k8s.io/kubernetes/pkg/apis/core/v1/helper

## Components

### Operator

The `operator` module (`scalingadvisor` binary) contains a k8s [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) based controller that wires up all other components of the scaling-advisor.  The operator config types are at packages under the `api` module under `api/config`. 

#### Operator CLI Configuration

The operator accepts the following CLI parameters:

| Parameter | Required | Description |
|---|---|---|
| `--control-namespace` | Yes | Namespace in the control cluster where `ScalingConstraint`, `ScalingAdvice`, `ScalingFeedback` objects reside. |
| `--control-kubeconfig` | Yes | Kubeconfig for the control cluster. |
| `--catalog-kubeconfig` | No | Kubeconfig for an optional catalog cluster hosting shared `NodeTemplateSet`, `InstanceTypeDefSet`, and `InstancePriceSet` objects. Falls back to `--target-kubeconfig` if absent. |
| `--target-kubeconfig` | Yes | Kubeconfig for the target (workload) cluster. Holds overrides for the catalog resources.|


#### Operator Reconciliation

The operator `Reconciler` invokes the [ScalingPlanner](#scalingplanner) during its reconciliation loop

```mermaid
---
title:  Operator Reconciliation
config:
  sequence:
    mirrorActors: false
---
sequenceDiagram
    participant crt as ControllerRuntime
    participant cc as ControlCluster
    participant cat as CatalogCluster
    participant tc as TargetCluster
    participant rc as Reconciler
    participant sp as ScalingPlanner
    participant lm as LifecycleManager

    crt->>+rc: invoke Reconcile
    note right of crt: object event, RequeueAfter,<br/>or SyncPeriod fallback

    rc->>+cc: get constraint objects
    note right of cc: ScalingConstraint, ScalingFeedback
    cc-->>-rc: objects
    rc->>+cat: get catalog objects
    note right of cat: NodeTemplateSet, InstanceTypeDefSet,<br/>InstancePriceSet<br/>(falls back to ControlCluster if absent)
    cat-->>-rc: objects
    rc->>+tc: get scheduling relevant objects
    note right of tc: Nodes, Pods, PVs, PVCs,<br/>StorageClasses, RuntimeClasses, CSINodes
    tc-->>-rc: objects
    rc->>rc: validate constraint and catalog objects
    alt validation fails
        rc->>cc: set ScalingConstraint.Status.Conditions (Accepted=False)
        rc-->>crt: return error
    else validation passes
        rc->>cc: set ScalingConstraint.Status.Conditions (Accepted=True)
        rc->>rc: build Request (ClusterSnapshot, ScalingConstraint, NodeTemplates, InstanceTypeDefs, InstancePrices, Memento)
        rc->>+sp: Prepare(ctx, request, batchWindowStart)
        sp-->>-rc: shouldPlan, preparedRequest, batchWindowStart
        rc->>rc: save preparedRequest.Memento, update batchWindowStart
        alt shouldPlan = true
            rc->>+sp: Plan(ctx, preparedRequest)
            loop deliver results
                sp-->>rc: Response (ScaleOutPlan, ScaleInPlan, Diagnostics)
            end
            sp-->>-rc: done
            rc->>rc: form ScalingAdvice
            rc->>cc: write ScalingAdvice to ctl cluster
            rc->>rc: clear batchWindowStart
            lm->>lm: watch ScalingAdvice, provision/decommission instances
            lm->>cc: write ScalingFeedback to ctl cluster
            rc-->>crt: RequeueAfter SyncPeriod
        else shouldPlan = false, batchWindowStart = zero
            rc-->>crt: RequeueAfter SyncPeriod
        else shouldPlan = false, batchWindow in progress
            rc-->>crt: RequeueAfter remaining
        end
    end
```

##### When does Operator invoke ScalingPlanner

**What triggers `Reconcile`**: any event on a control-cluster or target-cluster object watched by the operator, an explicit `ctrl.Result{RequeueAfter: X}` returned from a prior reconcile, or the `manager.Options.Cache.SyncPeriod` fallback (configurable, default `10s`).

**`Reconcile` flow**:

1. Fetch constraint objects (`ScalingConstraint`, `ScalingFeedback`) from the control cluster, catalog objects (`NodeTemplateSet`, `InstanceTypeDefSet`, `InstancePriceSet`) from the catalog cluster (falling back to the target cluster), and scheduling-relevant objects (`Nodes`, `Pods`, `PVs`, etc.) from the target cluster.
2. Validate constraint and catalog objects. On failure, set `ScalingConstraint.Status.Conditions (Accepted=False)` and return error. On success, set `Accepted=True`.
3. Build a `Request` from the fetched objects and the latest saved `Memento` (if any).
4. Invoke `ScalingPlanner.Prepare`:
    ```
    shouldPlan, preparedRequest, updatedBatchWindowStart = ScalingPlanner.Prepare(ctx, request, batchWindowStart)
    ```
    `batchWindowStart` is operator state (a field on the `Reconciler` struct), passed in and returned so `Prepare` remains stateless. Similarly, `Memento` is operator state — the `Reconciler` persists it between reconcile cycles and injects it into each `Request`; the planner never stores it. The operator always saves `preparedRequest.Memento` after this call regardless of `shouldPlan`.

    `Prepare` decides `shouldPlan` as follows:
    - **Scale-in path** (bypasses batch window): if any node has crossed its configured underutilization period, return `shouldPlan=true`, `batchWindowStart` unchanged.
    - **Scale-out path** (batch window applies):
        - If no unschedulable pods then clear `batchWindowStart` to zero, `shouldPlan=false`.
        - If unschedulable pods present and `batchWindowStart` is zero, then`batchWindowStart=now`, `shouldPlan=false`.
        -  If unschedulable pods present and `remaining = batchWindowDuration - (now - batchWindowStart)` > `0`, then  `shouldPlan=false`, `batchWindowStart` unchanged.
        - If unschedulable pods present and window expired, then `shouldPlan=true`, `batchWindowStart` to zero.
5. If `shouldPlan=true`: invoke `ScalingPlanner.Plan(ctx, preparedRequest)`, wait for `Response`s, write `ScalingAdvice`, update operator state, return `ctrl.Result{RequeueAfter: SyncPeriod}`.
6. If `shouldPlan=false`:
    - If `batchWindowStart` is zero (no pending scale-out): return `ctrl.Result{RequeueAfter: SyncPeriod}`.
    - If `batchWindowStart` is set (window in progress): return `ctrl.Result{RequeueAfter: remaining}` so the next reconcile fires exactly when the window expires. 


#### Operator Lifecycle Manager Interactions

The `Reconciler` writes a [`ScalingAdvice`](../api/core/v1alpha1/scalingadvice.go#L30) to the control cluster after each successful planner invocation. The **Lifecycle Manager** (LM, e.g. [machine-controller-manager](https://github.com/gardener/machine-controller-manager)) watches `ScalingAdvice` and responds by writing a companion [`ScalingFeedback`](../api/core/v1alpha1/scalingfeedback.go#L29) resource (same name and namespace, owner-referenced to the `ScalingAdvice`).

**Pacing rule**: the `Reconciler` will not produce a new `ScalingAdvice` until [`ScalingFeedback.Spec.AcknowledgedAt`](../api/core/v1alpha1/scalingfeedback.go#L53) is set on the current one or the current `ScalingAdvice` has been determined to be expired by the operator.

##### LM feedback protocol
1. On observing a new `ScalingAdvice`, the LM *immediately* sets `ScalingFeedback.Spec.AcknowledgedAt` to signal receipt and commitment to act.
2. As it begins to provisions/decommissions instances, it populates [`ScaleOutFeedback`](../api/core/v1alpha1/scalingfeedback.go#L63) and/or [`ScaleInFeedback`](../api/core/v1alpha1/scalingfeedback.go#L91). Note that the exact timing is dependent on the lifecycle manager implementation.
3. The `Reconciler` reads `ScalingFeedback` on the next reconcile tick (triggered by the feedback write) and incorporates the item-level results into the next `Request.Memento`.

##### Happy Path

```mermaid
---
title: LM Interactions - Happy Path
config:
  sequence:
    mirrorActors: false
---
sequenceDiagram
    participant ci as ControlCluster
    participant rc as Reconciler
    participant lm as LifecycleManager

    rc->>ci: write ScalingAdvice (ScaleOutPlan, ScaleInPlan)
    ci-->>lm: ScalingAdvice event
    note right of lm: LM commits to acting,<br/>Reconciler unblocked for next advice
    lm->>ci: write ScalingFeedback (AcknowledgedAt = now)
    lm->>lm: initiate provision and decommission instances
    lm->>ci: update ScalingFeedback (ScaleOutFeedback, ScaleInFeedback)
    ci-->>rc: ScalingFeedback event
    rc->>rc: read ScalingFeedback, update Memento
    rc->>ci: write next ScalingAdvice
```

##### Partial Failure and Backoff

When the LM cannot fulfil one or more [`ScaleOutItem`](../api/core/v1alpha1/scalingadvice.go#L108)s it records failure details per item in [`ScaleOutItemFeedback`](../api/core/v1alpha1/scalingfeedback.go#L72):

- [`FailCount`](../api/core/v1alpha1/scalingfeedback.go#L87): number of nodes that failed creation. The `Reconciler` uses this to decide whether to retain or discard the corresponding upcoming [`NodePlacement`](../api/core/v1alpha1/scalingadvice.go#L127) from the next `Request.Memento`.
- [`BackoffUntil`](../api/core/v1alpha1/scalingfeedback.go#L77): if set, the `Reconciler` will not include this `NodePlacement` in simulations or subsequent `ScaleOutPlan`s until the backoff expires.

```mermaid
---
title: LM Interactions - Partial Failure and Backoff
config:
  sequence:
    mirrorActors: false
---
sequenceDiagram
    participant ci as ControlCluster
    participant rc as Reconciler
    participant lm as LifecycleManager

    rc->>ci: write ScalingAdvice SA1 (ScaleOutPlan: items A, B)
    ci-->>lm: ScalingAdvice SA1 event
    lm->>ci: write ScalingFeedback (AcknowledgedAt = now)
    lm->>lm: provision item A: success
    lm->>lm: provision item B: failure
    lm->>ci: update ScalingFeedback (item B: FailCount=1, BackoffUntil=T)
    ci-->>rc: ScalingFeedback event
    rc->>rc: read feedback: backoff placement B until T,<br/>retain placement A in Memento
    rc->>ci: write ScalingAdvice SA2 (placement B excluded until backoff expires)
```
##### LM crashes

1. `ScalingAdvice` is produced by the operator.
1. LM recovers fast
    1. In such a case we proceed as normal as illustrated above
1. LM recovers slowly and the `ScalingFeedback.AcknowledgedAt` is not populated for long time
    1. Operator checks whether the latest `ScalingAdvice` is outdated, ie planner `Request` has diverged, then the outdated `ScalingAdvice` is marked as `Expired` (`Expired` condition on `ScalingAdviceStatus`) and then new `ScalingAdvice` is produced 


### ScalingPlanner
`ScalingPlanner` is a *functional*, **stateless** component that takes in
`plannerapi.Request` and delivers `plannerapi.Response` on a result channel. The
`ScalingPlanner` is invoked by the operator every time there is a change in the
`ScalingConstraint` or `ScalingFeedback` or the cluster snapshot (nodes, pods,
etc.) changes.


#### Scaling Planner Invocation Types

![ScalingPlanner - Invocation Types](./assets/types-invocation.svg)

| Type | Purpose |
|---|---|
| [`ScalingPlanner`](../planner/api/types.go#L280) | Interface; entry point: `Plan` returns a channel on which one or more `Response`s are delivered. |
| [`Request`](../planner/api/types.go#L34) | Everything the planner needs: cluster state, constraints, pricing, and cross-invocation memento. |
| [`RequestRef`](../planner/api/types.go#L25) | Stable identity (`ID` + `CorrelationID`) embedded in both `Request` and `Response` for correlation. |
| [`Response`](../planner/api/types.go#L74) | Single delivery on the result channel; carries a partial or final `ScaleOutPlan`/`ScaleInPlan` or a terminal `Error`. |
| [`ClusterSnapshot`](../planner/api/types.go#L93) | Scheduling-relevant cluster state at request time: nodes, pods, volumes, storage/priority/runtime classes. `Nodes` contains all registered nodes excluding those tainted with any of `NodePool.startupTaintKeys` (nodes still starting up are not yet schedulable and must not be counted as available capacity). |
| [`Memento`](../planner/api/types.go#L480) | Cross-invocation state passed back into the next `Request` to enable incremental planning. |
| [`ScaleInMemento`](../planner/api/types.go#L489) | Tracks per-node under-utilization timestamps so scale-in is only recommended after sustained under-use. |
| [`ScaleOutMemento`](../planner/api/types.go#L502) | Tracks node placements from prior accepted `ScaleOutPlan`s that are not yet `Ready`, avoiding double-counting. |
| [`ScalingConstraintSpec`](../api/core/v1alpha1/scalingconstraints.go#L50) | Operator-configured policy: node pools, and the three strategy selectors below. |
| [`PlannerStrategy`](../api/common/types.go#L49) | High-level ordering policy: whether to scale-in first, scale-out first, or consolidate into one pass. |
| [`SimulatorStrategy`](../api/common/types.go#L92) | Simulation topology: `SingleNodeMultiSim` (one node per sim, pick winner) vs `MultiNodeSingleSim` (scale many nodes in one sim). |
| [`NodeScoringStrategy`](../api/common/types.go#L152) | Algorithm used to rank candidate scale-out nodes: `LeastWaste` or `LeastCost`. |
| [`ScalingAdviceGenerationMode`](../api/common/types.go#L122) | `Incremental` delivers partial responses as simulations complete; `AllAtOnce` waits for ScalingPlanner.Plan to fully complete before writing `ScalingAdvice`. |

#### Planner Strategies 

##### PlanningStrategyConsolidatedScaleOut

`PlanningStrategyConsolidatedScaleOut` identifies scale-in candidate nodes via the [`ScaleInCandidateSelector`](#scaleincandidateselector), removes them from the view directly (without simulation), and then runs scale-out simulation(s) to produce a consolidated scale-in and scale-out plan.

- The `ScalingPlanner` calls `RemoveScaleInCandidates(ctx, view, candidates)` which:
  - Deletes the candidate nodes from the view.
  - Unschedules their pods (clears `pod.Spec.NodeName`).
  - For each PVC bound to a pod on a candidate node: clears `pv.Spec.ClaimRef` and resets the PVC phase to `Pending`, making the storage available for re-binding during scale-out simulation.
- The modified view is passed directly to `ScaleOutSimulator.Simulate`.
- The `ScaleInPlan` in the `Response` is built directly from the candidate list: no scale-in simulation is run.
- Success criteria: the simulation result is acceptable if the set of leftover unscheduled pods after scale-out simulation is a subset of the originally unscheduled pods (prior to invoking the planner).

Advantages:
- Best-fit node selection for pods displaced from underutilized nodes, since all such pods are explicitly unscheduled and compete equally in scale-out simulation.
- More responsive (in some cases) than `ScaleInThenScaleOut`: only scale-out simulations run, which may be optimised to a single simulation depending on `SimulatorStrategy`.

Disadvantages:
- Greater disruption period for pods on scaled-in nodes: they must wait for new scale-out nodes to become ready before being rescheduled.

```mermaid
---
title: PlanningStrategyConsolidatedScaleOut
config:
  sequence:
    mirrorActors: false
---
sequenceDiagram
    participant op as Operator
    participant sp as ScalingPlanner
    participant selector as ScaleInCandidateSelector
    participant sos as ScaleOutSimulator

    op->>+sp: Plan(request)
    sp->>sp: view = PopulateView(request)
    sp->>+selector: Select(ctx, view, args)
    selector-->>-sp: candidates []Node
    sp->>sp: RemoveScaleInCandidates(ctx, view, candidates)
    note right of sp: deletes nodes, unschedules pods,<br/>clears PV claimRefs, resets PVCs to Pending
    sp->>sp: scaleInPlan = BuildScaleInPlan(candidates)
    sp->>+sos: Simulate(ctx, simf, view)
    note right of sos: internal behavior depends on SimulatorStrategy
    sos-->>-sp: ScaleOutPlanResult (ScaleOutPlan)
    sp->>sp: BuildResponse(scaleInPlan, ScaleOutPlan)
    sp-->>-op: deliver Response
```


##### PlanningStrategyScaleInThenScaleOut

> [!NOTE]
> This Strategy will NOT be implemented in first delivery. But documented for posterity.

`PlanningStrategyScaleInThenScaleOut` executes scale-in simulation(s) first to produce scale-in plans, then executes scale-out simulation(s) to produce scale-out plans.

- Pure scale-in simulations involve removing underutilized nodes and verifying that the existing workload on those nodes can be rescheduled onto remaining nodes.
- Pure scale-out simulations involve adding nodes and verifying that unscheduled workload can be placed on them.
- The view produced by the last successful scale-in simulation is passed to the `ScaleOutSimulator`.
- If `Request.AdviceGenerationMode` is `Incremental`, a partial `Response` carrying only the `ScaleInPlan` is delivered to the operator before scale-out begins.

Advantages:
- Explicit scale-in simulation honours PDBs and reschedules existing pods onto already-existing nodes, minimising disruption (no new nodes needed for those pods).
- Better suited to instance quota limits since scale-in runs first.

Disadvantages:
- Prioritising no-disruption over better packing: an underutilized node whose pods cannot move to existing nodes is left in place, even if a new scale-out node would fit them better.
- Scale-out plans are delayed until all scale-in simulations complete, so latency-sensitive workloads waiting for immediate scale-out are affected.

```mermaid
---
title: PlanningStrategyScaleInThenScaleOut
config:
  sequence:
    mirrorActors: false
---
sequenceDiagram
    participant op as Operator
    participant sp as ScalingPlanner
    participant selector as ScaleInCandidateSelector
    participant sis as ScaleInSimulator
    participant sos as ScaleOutSimulator

    op->>+sp: Plan(request)
    sp->>sp: view = PopulateView(request)
    sp->>+selector: Select(ctx, view, args)
    selector-->>-sp: candidates []Node
    sp->>sp: annotate candidates in view with AnnotationScaleInCandidate=index
    sp->>+sis: Simulate(ctx, simf, view)
    note right of sis: internal behavior depends on SimulatorStrategy
    sis-->>-sp: ScaleInPlanResult (ScaleInPlan, finalView)
    sp->>sp: BuildResponse(ScaleInPlan)
    opt AdviceGenerationMode == Incremental
        sp-->>op: deliver partial Response (ScaleInPlan only)
    end

    sp->>+sos: Simulate(ctx, simf, view)
    note right of sos: internal behavior depends on SimulatorStrategy
    sos-->>-sp: ScaleOutPlanResult (ScaleOutPlan)
    sp->>sp: BuildResponse(ScaleOutPlan)
    sp-->>-op: deliver Response (ScaleOutPlan)
```

#### ScaleInCandidateSelector

The `ScaleInCandidateSelector` identifies candidate nodes considering annotations, PDBs, utilization thresholds, pool min limits, pool priorities, and `NodeTemplateRef` priorities - without running any simulation involving the `kube-scheduler`. It is invoked by the `ScalingPlanner` before considering either `PlanningStrategyConsolidatedScaleOut` or `PlanningStrategyScaleInThenScaleOut`.

```mermaid
---
title: ScaleInCandidateSelector - Selection
config:
  sequence:
    mirrorActors: false
---
sequenceDiagram
    participant sp as ScalingPlanner
    participant sel as ScaleInCandidateSelector
    participant pdb as PDBTracker

    sp->>+sel: Select(ctx, view, args)
    sel->>sel: list all nodes from view
    sel->>sel: filter nodes with AnnotationDoNotDisrupt
    sel->>sel: filter nodes with pods having AnnotationDoNotDisrupt
    sel->>sel: filter nodes violating PoolConfigs.MinLimits
    sel->>sel: retain underutilized nodes (per PoolConfigs.UtilizationThresholds) considering ScaleInMemento
    sel->>sel: sort by PoolPriority asc, then TemplatePriority asc, then utilization asc
    sel->>+pdb: newPDBTracker(pdbs)
    pdb-->>-sel: PDBTracker
    loop for each candidate node
        sel->>+pdb: CanRemovePods(nodePods)
        pdb-->>-sel: ok, blockingPod
        alt ok
            sel->>pdb: ConsumePods(nodePods)
            sel->>sel: append to accepted
        end
    end
    sel-->>-sp: candidates []Node
```

```mermaid
---
title: ScaleInCandidateSelector - Types
---
classDiagram
    class ScaleInCandidateSelector {
        <<interface>>
        +Select(ctx, View, ScaleInCandidateSelectorArgs) ([]Node, error)
    }
    class ScaleInCandidateSelectorArgs {
        +Memento ScaleInMemento
        +PoolConfigs map~string,NodePoolScaleInConfig~
    }
    class ScaleInMemento {
        +LastIdentifiedUnneededNodes map~string,Time~
        +LastUnderutilizedSinceNodes map~string,Time~
    }
    class NodePoolScaleInConfig {
        +UtilizationThresholds map~ResourceName,float64~
        +MinLimits ResourceList
        +TemplatePriorities map~string,int32~
        +Priority int32
    }

    ScaleInCandidateSelector ..> ScaleInCandidateSelectorArgs : uses
    ScaleInCandidateSelectorArgs *-- ScaleInMemento : Memento
    ScaleInCandidateSelectorArgs *-- NodePoolScaleInConfig : PoolConfigs
```

| Field | Purpose |
|---|---|
| `PoolConfigs` | Per-pool configuration keyed by pool name. |
| `NodePoolScaleInConfig.UtilizationThresholds` | Per-resource fractions (0-1); a node is underutilized only if all configured resources are below their threshold. |
| `NodePoolScaleInConfig.MinLimits` | Minimum total allocatable resources for the pool; a candidate is rejected if its removal would drop the pool below this minimum. |
| `NodePoolScaleInConfig.TemplatePriorities` | Priority per node template name within the pool; lower value is preferred for scale-in. |
| `NodePoolScaleInConfig.Priority` | Pool-level priority; lower value is preferred for scale-in over higher-priority pools. |

### Simulators

Simulators are facades that orchestrate one or more `Simulation` runs against sandboxed `minkapi` views to produce scaling plans. 

- The `ScaleOutSimulator` for `single-node-multi-sim` creates a `ScaleOutNodeTemplate` per `(NodePool, NodeTemplate, Zone)` triple, groups them by priority, and runs stabilization cycles to determine which node placements best absorb unscheduled pods.
- The `ScaleInSimulator` drives scale-in: it processes annotated candidate nodes sequentially, promoting sandbox views on success to build up a `ScaleInPlan`.
- Simulators delegate pod-to-node scoring to a `NodeScorer` and simulation execution to a `Simulation` obtained via a `SimulationFactory` residing in the `simulation` module. 


#### Types

![ScalingPlanner - Simulator Types](./assets/types-simulator.svg)

| Type | Purpose |
|---|---|
| [`ScalingPlannerArgs`](../planner/api/types.go#L264) | Constructor input; wires together the `ViewAccess`, `SchedulerLauncher`, factories, and config the planner depends on. |
| [`SimulatorFactory`](../planner/api/types.go#L314) | Creates `ScaleOutSimulator` and `ScaleInSimulator` instances for a given `SimulatorArgs`. |
| [`SimulatorArgs`](../planner/api/types.go#L321) | Constructor input for simulators: view access, scheduler launcher, node scorer, request, strategy, and config. |
| [`ScaleOutSimulator`](../planner/api/types.go#L374) | Runs scale-out simulations against a sandboxed `View` and streams `ScaleOutPlanResult`s on a channel. |
| [`ScaleInSimulator`](../planner/api/types.go#L384) | Runs scale-in simulations against a sandboxed `View` and streams `ScaleInPlanResult`s on a channel. |
| [`ScaleOutPlanResult`](../planner/api/types.go#L434) | Single delivery from `ScaleOutSimulator.Simulate`; contains a `ScaleOutPlan` or a terminal `Error`. |
| [`ScaleInPlanResult`](../planner/api/types.go#L442) | Single delivery from `ScaleInSimulator.Simulate`; contains a `ScaleInPlan`, updated `ScaleInMemento`, and the post-simulation `View`. |
| [`NodeScorer`](../planner/api/types.go#L208) | Scores and selects the winning simulated node; strategy is determined by `NodeScoringStrategy` in the request. |
| [`NodeScorerArgs`](../planner/api/types.go#L217) | Input to `NodeScorer.Score`: placement, pod assignments, leftover unscheduled pods, pricing, and resource weights. |
| [`NodeScore`](../planner/api/types.go#L238) | Output of `NodeScorer.Score`: numeric score, placement, scaled node resource info, and unscheduled pods. |
| [`SimulatorConfig`](../planner/api/types.go#L254) | Static config: under-utilization duration and max parallel simulations. |
| [`ScaleOutNodeTemplate`](../simulation/api/types.go#L146) | Encapsulates all data needed to virtually scale out one node: embeds `NodePlacement` (`PoolName`, `TemplateName`, `InstanceType`, `Region`, `Zone`), full capacity (`BaseResources`, `ExtendedResources`, `ReservedResources`), `Labels`, `Taints`, optional `ResourceLimits`, and `GroupKey`. Depending on SimulatorStrategy, either multiple simulations are created per `ScaleOutNodeTemplate` or multiple nodes in one simulation. |
| [`GroupKey`](../simulation/api/types.go#L117) | Composite sort key `(NodePoolPriority, NodeTemplatePriority)` used to group and order simulation groups. |
| [`SimulationGroup`](../simulation/api/types.go#L193) | A group of `Simulation`s sharing the same `GroupKey`; runs them concurrently and returns all `SimulationResult`s. |
| [`ScaleOutSimGroupPassScores`](../planner/api/types.go#L453) | Scored results of one pass within a group: winner `NodeScore`, winner `Node`, and all scores. |
| [`ScaleOutSimGroupCycleResult`](../planner/api/types.go#L463) | Accumulated result of a group's stabilization cycle: winner node scores, leftover pods, pass count, and next view. |

#### ScaleOutSimulator 

##### SimulatorStrategySingleNodeMultiSim (single-node-multi-sim)

`SimulatorStrategySingleNodeMultiSim` runs one simulation per `(NodePool, NodeTemplate, Zone)` triplet. Each simulation scales exactly one virtual node of that triplet into a sandbox minkapi view and lets the `kube-scheduler` place pending pods onto it. After all simulations in a priority group complete one pass, the `NodeScorer` picks the winner; that winner's sandbox becomes the base view for the next pass. The group repeats passes until no pods remain unscheduled or no winner is found. The simulator then advances to the next priority group and repeats.

Key concepts:

- **`ScaleOutNodeTemplate`**: encapsulates all data needed to virtually scale out one node. Embeds `NodePlacement` (`PoolName`, `TemplateName`, `InstanceType`, `Region`, `Zone`) plus full capacity info (`BaseResources`, `ExtendedResources`, `ReservedResources`), `Labels`, `Taints`, optional `ResourceLimits`, and `GroupKey`. One simulation is created per `ScaleOutNodeTemplate`.
- **`SimulationGroup`**: simulations are grouped by `(NodePool.Priority, NodeTemplate.Priority)`. Lower priority value is preferred: groups are processed in ascending priority order.
- **Pass**: one execution of all simulations in a group; each simulation runs concurrently against a sandbox of the current `groupPassView`.
- **Stabilization cycle**: repeated passes of a group until either no unscheduled pods remain or no winner emerges. The winner's sandbox view becomes the `groupPassView` for the next pass.
- **`ScaleOutSimGroupPassScores`**: the scored results of one pass: winner `NodeScore` plus all scores.
- **`ScaleOutSimGroupCycleResult`**: accumulated result of all passes for one group: winner node scores, leftover unscheduled pods, pass count, and the `NextGroupPassView` handed to the next group.

![ScaleOutSimulator - SimulatorStrategySingleNodeMultiSim](./assets/single-node-multi-sim.svg)

##### SimulatorStrategyMultiNodeSingleSim (multi-node-single-sim)

`SimulatorStrategyMultiNodeSingleSim` represents a simulator strategy in which the `ScaleOutSimulator` runs a single simulation by scaling multiple nodes for all combinations of NodePool, NodeTemplate and AvailabilityZone, ie for all `ScaleOutNodeTemplate`s

This section will be filled up when the PR for this feature is raised in the future. 

### MinKAPI

`minkapi` is a minimal in-memory Kubernetes API layer used by the `ScalingAdvisor` that holds the scheduling-relevant state of a cluster such that simulations can be run against it, all without touching the real cluster.

#### Design

The central abstraction is a [`View`](../minkapi/api/types.go): a named, typed store of Kubernetes objects (Nodes, Pods, PVs, PVCs, StorageClasses, Events, and more) that exposes the full CRUD + Watch surface the `kube-scheduler` expects. There are two `ViewType`s:

- **Base view**: a persistent, initially empty view created once at startup by `ViewAccess`. The base view itself does not hold cluster snapshot data directly. Instead, for each planning request, the planner creates a sandbox *request view* over the base view and populates it with the current `ClusterSnapshot` (nodes, pods, PVs, PVCs, etc.). The base view survives across planning requests and can be used to hold static unchanging objects.
- **Sandbox view**: a private overlay created on top of any delegate view (the base, or another sandbox). All mutations (creates, updates, deletes) are confined to the sandbox's own in-memory stores; reads fall through to the delegate when the object is not present locally. Deletions are implemented via tombstones so the delegate's copy is never touched. Sandbox views are short-lived: one is created per simulation run and discarded afterwards.

`ViewAccess` is the top-level MinkAPI facade to manages view lifecycle: `GetBaseView()` returns the singleton base view; `GetSandboxView(ctx, name)` creates or returns a named sandbox over the base; `GetSandboxViewOverDelegate(ctx, name, delegate)` creates a sandbox over an arbitrary delegate view (used to layer simulation passes on top of each other).

```go
type ViewAccess interface {
	io.Closer
	GetBaseView() View
	GetSandboxView(ctx context.Context, name string) (View, error)
	GetSandboxViewOverDelegate(ctx context.Context, name string, delegateView View) (View, error)
}

```
#### Key Types

```mermaid
classDiagram
    class View {
        <<interface>>
        +GetName() string
        +GetType() ViewType
        +GetClientFacades(ctx, mode) ClientFacades
        +GetResourceStore(gvk) ResourceStore
        +GetEventSink() EventSink
        +CreateObject(ctx, gvk, obj, opts) Object
        +GetObject(ctx, gvk, name) Object
        +UpdateObject(ctx, gvk, obj, opts)
        +UpdatePodNodeBinding(ctx, name, binding) Pod
        +DeleteObject(ctx, gvk, name)
        +ListNodes(ctx, names) []Node
        +ListPods(ctx, criteria) []Pod
        +WatchObjects(ctx, gvk, ..., cb)
        +GetObjectChangeCount() int64
    }
    class ViewAccess {
        <<interface>>
        +GetBaseView() View
        +GetSandboxView(ctx, name) View
        +GetSandboxViewOverDelegate(ctx, name, delegate) View
        +Close()
    }
    class Server {
        <<interface>>
        +Start(ctx)
        +Stop(ctx)
        +GetBaseView() View
        +GetSandboxView(ctx, name) View
    }
    class ResourceStore {
        <<interface>>
        +Add(ctx, obj, opts)
        +Get(ctx, name) Object
        +Update(ctx, obj, opts)
        +Delete(ctx, name, opts)
        +List(ctx, criteria) ObjectList
        +Watch(ctx, version, ns, sel, cb)
        +GetVersionCounter() *atomic.Int64
    }
    class EventSink {
        <<interface>>
        +List() []Event
        +Reset()
    }
    class ClientFacades {
        +Client kubernetes.Interface
        +DynClient dynamic.Interface
        +InformerFactory SharedInformerFactory
        +Mode ClientAccessMode
    }
    class ViewType {
        <<enumeration>>
        ViewTypeBase
        ViewTypeSandbox
    }
    class MatchCriteria {
        +Namespace string
        +Names Set~string~
        +ExcludeNames Set~string~
        +LabelSelector labels.Selector
    }

    ViewAccess --> View : manages
    Server --|> ViewAccess : extends
    View --> ResourceStore : per-GVK store
    View --> EventSink : scheduler events
    View --> ClientFacades : vends
    View --> ViewType : has type
```

| Type | Purpose |
|---|---|
| [`View`](../minkapi/api/types.go) | Named, typed in-memory store; exposes CRUD + Watch per GVK plus typed helpers (`ListNodes`, `ListPods`, `UpdatePodNodeBinding`). |
| [`ViewAccess`](../minkapi/api/types.go) | Lifecycle manager for views; creates base and sandbox views and provides `GetSandboxViewOverDelegate`. |
| [`ResourceStore`](../minkapi/api/types.go) | Per-GVK store backing a `View`; supports versioned watch, tombstones, and label-selector filtering. |
| [`EventSink`](../minkapi/api/types.go) | Collects `events.v1` events emitted by the `kube-scheduler`; used by `RunState.Track` to detect pod bindings. |
| [`ClientFacades`](../minkapi/api/types.go) | Bundle of `kubernetes.Interface` and `dynamic.Interface` backed by a `View`. |
| [`Server`](../minkapi/api/types.go) | HTTP server interface; `InMemServer` implements it via `Start`/`Stop`/`GetSandboxView`. |

---

#### In-memory Kubernetes Clients

Each `View` exposes a [`ClientFacades`](../minkapi/api/types.go) bundle - the `kubernetes.Interface` and `dynamic.Interface` that the `kube-scheduler` talks to - in two modes:

- `ClientAccessModeInMemory`: all calls are handled inside the same process, backed directly by the view's stores. No TCP stack involved.
- `ClientAccessModeNetwork`: calls go over HTTP to the embedded [`InMemServer`](../minkapi/server/server.go) (see below). Used when the scheduler binary must connect via kubeconfig.

In the `SingleNodeMultiSim` strategy the scheduler is always launched in-process via `ClientAccessModeInMemory`, which avoids the overhead of the HTTP server. The HTTP server is optional and used mainly for standalone testing or integration with out-of-process schedulers.

#### HTTP Server (`InMemServer`)

[`InMemServer`](../minkapi/server/server.go) wraps a `ViewAccess` with a standard Go `net/http` server that speaks a subset of the Kubernetes API (`core/v1`, `storage`, `events`). Each `View` - base and sandbox - gets its own route prefix and `http.ServeMux`, all registered under the root mux. The server generates a kubeconfig and a kube-scheduler config pointing at the correct URL so external processes can connect without further configuration.


### Simulation

The `simulation` module is responsible for running individual scheduling simulations against `minkapi` views. A `Simulation` creates virtual scale-out nodes in a sandbox view, launches an embedded `kube-scheduler` against that view, and tracks scheduler events until all pending pods are placed or the run stabilizes.

#### Design

[`Simulation`](../simulation/api/types.go) is the core interface. The default implementation ([`defaultSimulation`](../simulation/scaleout/simulation.go)) performs three steps inside `Run(ctx, view)`:

1. **`CreateSimulationNodes`**: materializes one or more `ScaleOutSimNode` objects (and associated `CSINode`s) into the sandbox view. Node names encode the run number, pool, template, and availability zone for traceability.
2. **`launchSchedulerForSimulation`**: obtains in-memory `ClientFacades` from the view and calls `SchedulerLauncher.Launch`. The scheduler runs concurrently against the same sandbox view.
3. **`workAndTrackUntilStabilized`**: polls the view's `EventSink` in a tight loop via `RunState.Track`. Each iteration also does volume work (`ProvisionAndBindVolumesFoSelectedClaimsInWFFC`, `FinalizeStaticBindingsForSelectedClaimsInWFFC`) to unblock storage-bound pods. The loop ends when no new scheduler events have arrived for `MaxUnchangedTrackAttempts` consecutive polls, all pending pods are scheduled, or the context is cancelled.

[`RunState`](../simulation/scaleout/runstate.go) holds all mutable tracking state for one simulation run: the set of unscheduled pods, the set of scale-out nodes created, pod-to-node assignments, and counters for track attempts.

#### SimulationFactory

[`SimulationFactory`](../simulation/api/types.go) is a simple factory interface with a single `New(SimulationArgs)` method. The default implementation ([`factory.New()`](../simulation/factory/factory.go)) always produces a `defaultSimulation`. The interface exists so callers (the `ScaleOutSimulator`) can swap in alternative implementations (e.g. a SAT/MIP solver) without code changes.

#### Key Types

```mermaid
classDiagram
    class Simulation {
        <<interface>>
        +Name() string
        +Status() ActivityStatus
        +GroupKey() GroupKey
        +Run(ctx, view) SimulationResult
        +Reset()
    }
    class SimulationFactory {
        <<interface>>
        +New(args SimulationArgs) Simulation
    }
    class SimulationArgs {
        +Name string
        +NodeTemplates []ScaleOutNodeTemplate
        +Config SimulationConfig
        +SchedulerLauncher SchedulerLauncher
        +RunCounter *atomic.Uint32
        +FallbackCSINodeSpecs map~string~CSINodeSpec
        +Strategy SimulatorStrategy
        +TraceDir string
    }
    class SimulationConfig {
        +TrackPollInterval Duration
        +MaxUnchangedTrackAttempts int
        +BindVolumeClaimsForImmediateMode bool
    }
    class SimulationResult {
        +Name string
        +View minkapi.View
        +ScaleOutItems []ScaleOutItem
        +NodePodAssignments []NodePodAssignment
        +OtherNodePodAssignments []NodePodAssignment
        +LeftoverUnscheduledPods []NamespacedName
    }
    class ScaleOutNodeTemplate {
        +NodePlacement (inline)
        +BaseResources ResourceList
        +ExtendedResources ResourceList
        +ReservedResources ResourceList
        +ResourceLimits *ResourceLimits
        +Labels map~string~string
        +Taints []Taint
        +Architecture string
        +GroupKey GroupKey
    }
    class GroupKey {
        +NodePoolPriority int32
        +NodeTemplatePriority int32
    }
    class SimulationGroup {
        <<interface>>
        +Name() string
        +Key() GroupKey
        +GetSimulations() []Simulation
        +AddSimulation(sim)
        +Run(ctx, getViewFn) []SimulationResult
    }
    class SchedulerLauncher {
        <<interface>>
        +Launch(ctx, params) SchedulerHandle
    }
    class SchedulerLaunchParams {
        +ClientFacades
        +EventSink
    }
    class ActivityStatus {
        <<enumeration>>
        Pending
        Running
        Success
        Failure
    }

    SimulationFactory ..> Simulation : creates
    Simulation --> GroupKey : has
    Simulation --> ActivityStatus : has status
    Simulation --> SimulationResult : returns
    SimulationGroup --> Simulation : groups
    SimulationGroup --> GroupKey : keyed by
    SimulationArgs --> ScaleOutNodeTemplate : contains
    SimulationArgs --> SimulationConfig : contains
    SimulationArgs --> SchedulerLauncher : uses
    ScaleOutNodeTemplate --> GroupKey : has
    SimulationResult --> ScaleOutNodeTemplate : references via View
    SchedulerLauncher ..> SchedulerLaunchParams : takes
```

| Type | Purpose |
|---|---|
| [`Simulation`](../simulation/api/types.go) | Core interface: `Run(ctx, view)` executes a simulation to completion and returns `SimulationResult`. |
| [`SimulationFactory`](../simulation/api/types.go) | Factory for `Simulation` instances; `New(SimulationArgs)` is the only method. |
| [`SimulationArgs`](../simulation/api/types.go) | Constructor arguments: `NodeTemplates`, `Config`, `SchedulerLauncher`, `RunCounter`, `FallbackCSINodeSpecs`, and `Strategy`. |
| [`SimulationConfig`](../simulation/api/types.go) | Per-simulation config: `TrackPollInterval`, `MaxUnchangedTrackAttempts`, `BindVolumeClaimsForImmediateMode`. |
| [`SimulationResult`](../simulation/api/types.go) | Output of a completed run: `ScaleOutItems` (placements + deltas), `NodePodAssignments` (scale-out nodes), `OtherNodePodAssignments` (existing nodes that also absorbed pods), `LeftoverUnscheduledPods`, and the `View` the run was performed against. |
| [`RunState`](../simulation/scaleout/runstate.go) | Internal per-run mutable state: unscheduled pod map, scale-out node map, placement counts, pod-to-node assignments, and stabilization counters. |
| [`SchedulerLauncher`](../simulation/api/types.go) | Launches an embedded `kube-scheduler` instance; returns a `SchedulerHandle` (`io.Closer`) for lifecycle management. |
| [`SchedulerLaunchParams`](../simulation/api/types.go) | Input to `Launch`: `ClientFacades` and `EventSink` from the simulation's sandbox view. |
