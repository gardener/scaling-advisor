# Benchmark Report Schema

Documentation for the JSON report produced by `scalebench exec`.

## metadata

| Field | Description |
|-------|-------------|
| `startTime` | When the benchmark run began |
| `endTime` | When the benchmark run finished (after all events collected and report written) |
| `totalRunDuration` | Wall-clock duration of the full benchmarking run |
| `scalerName` | Scaler under test (`cluster-autoscaler` or `karpenter`) |
| `scalerVersion` | Image tag/version used |
| `snapshotFile` | Path to the cluster snapshot input |
| `clusterState` | Cluster state before and after scaling (see below) |
| `scalingTime` | Fine-grained timing breakdown (see below) |
| `eventsSummary` | Summary of collected events (see below) |

### clusterState.before / clusterState.after

| Field | Description |
|-------|-------------|
| `nodeCount` | Total number of nodes |
| `scheduledPods` | Pods assigned to a node |
| `unscheduledPods` | Pods without a node assignment |

### scalingTime

| Field | Description |
|-------|-------------|
| `firstFailedScheduling` | Timestamp of first pod that couldn't be placed |
| `firstNodeCreated` | Timestamp when scaler created its first new node |
| `lastPodScheduled` | Timestamp when the final unscheduled pod got a node |
| `reactionTime` | Time from first failure to first node creation |
| `schedulingTime` | Time from first node creation to last pod scheduled |
| `totalDuration` | End-to-end: first failure to last pod scheduled |

### eventsSummary

| Field | Description |
|-------|-------------|
| `firstEventTime` | Timestamp of the earliest event collected |
| `lastEventTime` | Timestamp of the latest event collected |
| `totalCount` | Total number of events captured |
| `countByType` | Map of event type to count (see events section for type descriptions) |
| `instanceTypes` | Map of instance type to count of nodes created with that type |
| `unschedulablePods` | Number of pods that could not be scheduled on any node group (CA: NotTriggerScaleUp) |

## metrics

Time-series resource usage of the scaler container, streamed from Docker at ~1s intervals.

| Field | Description |
|-------|-------------|
| `timestamp` | When the sample was taken |
| `containers[].name` | Container name |
| `containers[].stats.cpuMillicores` | CPU usage in millicores (1000 = 1 core) |
| `containers[].stats.memoryMi` | Total memory usage (includes page cache) in MiB |
| `containers[].stats.memoryRSSMi` | Resident set size (actual RAM, excludes page cache) in MiB |
| `containers[].stats.memoryMaxUsageMi` | Peak memory since container start in MiB |
| `containers[].stats.memoryLimitMi` | Container memory limit in MiB |
| `containers[].stats.cpuThrottledPeriods` | CPU scheduling periods where the container was throttled |
| `containers[].stats.cpuTotalPeriods` | Total CPU scheduling periods elapsed |
| `containers[].stats.cpuThrottledTimeNs` | Total nanoseconds the container was CPU-throttled |
| `containers[].stats.pids` | Number of processes/threads in the container |

## events

Chronological log of scaling activity. Each event has: `timestamp`, `type`, `name`, `namespace` (optional), `details` (optional).

### Common events (both scalers)

| Type | Source | Description |
|------|--------|-------------|
| `FailedScheduling` | Event watch | Timestamp of first pod that couldn't be placed (recorded once) |
| `NodeCreated` | Node watch | A new node appeared in the cluster (scaler provisioned it). Details show instance type |
| `PodScheduled` | Pod watch | A previously-unscheduled pod's `NodeName` was set. Details show target node |

### Cluster Autoscaler events

| Type | Source | Description |
|------|--------|-------------|
| `TriggeredScaleUp` | Event watch | CA determined this pod needs a new node and triggered scale-up |
| `ScaledUpGroup` | Event watch | CA scaled up a node group (increased desired count). Details show the group and new size |
| `NotTriggerScaleUp` | Event watch | CA determined this pod cannot fit on any node group. Pod is marked as permanently unschedulable |

### Karpenter events

| Type | Source | Description |
|------|--------|-------------|
| `Nominated` | Event watch | Karpenter nominated a pod for scheduling on a new or existing nodeclaim/node |
| `NoCompatibleInstanceTypes` | Event watch | NodePool requirements filtered out all compatible instance types |
