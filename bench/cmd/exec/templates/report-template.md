# Benchmark Report Schema

Documentation for the JSON reports produced by `scalebench exec`.

Three files are written to the logs directory:

| File | Description |
|------|-------------|
| `scaler-report.json` | Run metadata, cluster state, scaling timeline, and summary |
| `scaler-events.json` | Chronological timeline of scaling activity |
| `metrics/` | Per-container CSV time-series resource usage of the scaler |

---

## scaler-report.json

### Top-level fields

| Field | Description |
|-------|-------------|
| `startTime` | When the benchmark run began |
| `endTime` | When the benchmark run finished |
| `totalRunDuration` | Wall-clock duration of the full benchmarking run |
| `scalerName` | Scaler under test (`cluster-autoscaler` or `karpenter`) |
| `scalerVersion` | Image tag/version used |
| `snapshotFile` | Path to the cluster snapshot input |
| `summary` | Structured summary (see below) |

### summary

| Field | Description |
|-------|-------------|
| `scalingTimeline` | Scaling timeline milestones and durations |
| `clusterState` | Cluster state before and after scaling |
| `events` | Event count information |
| `nodes` | Node scaling information |
| `pods` | Pod scheduling information |

### summary.scalingTimeline

| Field | Description |
|-------|-------------|
| `firstFailedScheduling` | Timestamp of first pod that couldn't be placed |
| `firstNodeCreated` | Timestamp when scaler created its first new node |
| `lastPodResolved` | Timestamp when the last pod was either scheduled or deemed unschedulable |
| `reactionTime` | Time from first failure to first node creation |
| `schedulingTime` | Time from first node creation to last pod scheduled |
| `totalDuration` | End-to-end: first failure to last pod scheduled |

### summary.events

| Field | Description |
|-------|-------------|
| `totalCount` | Total number of events captured |
| `countByType` | Map of event type to count |

### summary.nodes

| Field | Description |
|-------|-------------|
| `totalCreated` | Number of new nodes created during scaling |
| `instanceTypes` | Map of instance type to count of nodes created with that type |

### summary.clusterState.before / .after

| Field | Description |
|-------|-------------|
| `nodeCount` | Total number of nodes |
| `scheduledPods` | Pods assigned to a node |
| `unscheduledPods` | Pods without a node assignment |

### summary.pods

| Field | Description |
|-------|-------------|
| `unschedulablePods` | Number of pods that could not be scheduled |
| `schedulingLatency` | Per-pod scheduling latency percentiles (see below) |
| `schedulingDurations` | Map of namespaced pod name to individual scheduling duration |
| `failures` | Pods that failed to scale (see below) |

### summary.pods.schedulingLatency

Per-pod latency from pod creation to pod scheduled, computed across all scheduled pods.

| Field | Description |
|-------|-------------|
| `p50` | 50th percentile (median) scheduling latency |
| `p90` | 90th percentile scheduling latency |
| `p99` | 99th percentile scheduling latency |
| `max` | Maximum scheduling latency observed |

### summary.pods.failures[]

Pods that could not be scheduled by the scaler.

| Field | Description |
|-------|-------------|
| `podName` | Name of the pod that failed |
| `reason` | Why it failed (`NotTriggerScaleUp` or `NoCompatibleInstanceTypes`) |
| `details` | Event message explaining the failure |

---

## scaler-events.json

Chronological timeline of scaling activity.

Each event has: `timestamp`, `type`, `source`, `name`, `namespace` (optional), `details` (optional).

### Event types in the timeline

| Type | Source | Description |
|------|--------|-------------|
| `FailedScheduling` | Event watch | First pod that couldn't be placed (recorded once) |
| `NodeCreated` | Node watch | A new node appeared in the cluster. Details show instance type |
| `PodScheduled` | Pod watch | A previously-unscheduled pod got assigned a node. Details show target node |

### Cluster Autoscaler tracked events

| Reason | Source | Description |
|--------|--------|-------------|
| `TriggeredScaleUp` | `cluster-autoscaler` | CA determined a pod needs a new node and triggered scale-up |
| `ScaledUpGroup` | `cluster-autoscaler` | CA scaled up a node group. Details show group and new size |
| `NotTriggerScaleUp` | `cluster-autoscaler` | CA determined this pod cannot fit on any node group (marks unschedulable) |

### Karpenter tracked events

| Reason | Source | Description |
|--------|--------|-------------|
| `Nominated` | `karpenter` | Karpenter nominated a pod for scheduling on a new nodeclaim |
| `Launched` | `karpenter` | Karpenter launched a new nodeclaim/node |
| `NoCompatibleInstanceTypes` | `karpenter` | NodePool requirements filtered out all compatible instance types |
| `FailedScheduling` | `karpenter` | Karpenter could not schedule the pod (marks unschedulable) |

---

## metrics/ (CSV)

Per-container CSV files with time-series resource usage, streamed from Docker at ~1s intervals.

| Column | Description |
|--------|-------------|
| `timestamp` | When the sample was taken (RFC3339) |
| `cpu_millicores` | CPU usage in millicores (1000 = 1 core) |
| `memory_mi` | Total memory usage (includes page cache) in MiB |
| `memory_rss_mi` | Resident set size (actual RAM, excludes page cache) in MiB |
| `memory_max_usage_mi` | Peak memory since container start in MiB |
| `memory_limit_mi` | Container memory limit in MiB |
| `cpu_throttled_periods` | CPU scheduling periods where the container was throttled |
| `cpu_total_periods` | Total CPU scheduling periods elapsed |
| `cpu_throttled_time_ns` | Total nanoseconds the container was CPU-throttled |
| `pids` | Number of processes/threads in the container |
