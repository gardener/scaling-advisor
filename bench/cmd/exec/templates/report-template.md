# Benchmark Report Schema

Documentation for the JSON report produced by `scalebench exec`.

## metadata

| Field | Description |
|-------|-------------|
| `startTime` | When the benchmark run began |
| `scalerName` | Scaler under test (`cluster-autoscaler` or `karpenter`) |
| `scalerVersion` | Image tag/version used |
| `snapshotFile` | Path to the cluster snapshot input |
| `clusterState` | Cluster state before and after scaling (see below) |
| `scalingTime` | Fine-grained timing breakdown (see below) |

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

Chronological log of scaling activity.

| Type | Description |
|------|-------------|
| `FailedScheduling` | A pod couldn't be placed. Details show the reason (e.g. "Insufficient cpu") |
| `NodeCreated` | Scaler provisioned a new node. Details show instance type |
| `PodScheduled` | A previously-unscheduled pod was assigned to a node. Details show target node |

Each event has: `timestamp`, `type`, `name`, `namespace` (optional), `details` (optional).
