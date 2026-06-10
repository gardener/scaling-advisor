// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gardener/scaling-advisor/api/planner"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/k8s/watcher"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// ExecScaler is the interface that every scaler backend must implement to
// participate in a benchmark run.
type ExecScaler interface {
	// DeployNodes creates the nodes that can satisfy the scaler-specific
	// requirements (i.e. have specific annotations, id etc) and then deploys
	// them in the KWOK cluster.
	DeployNodes(ctx context.Context, cfg *envconf.Config, snapshot *planner.ClusterSnapshot) error

	// DeployScalerData creates the scaler-specific Kubernetes objects (CRDs,
	// ConfigMaps, NodePools, etc.) in the KWOK cluster.
	DeployScalerData(ctx context.Context, cfg *envconf.Config, scenarioDir string) error

	// GetScalerKWOKTemplatePath returns the embedded-FS path to the
	// kwokctl configuration template for this scaler.
	GetScalerKWOKTemplatePath() string

	// CheckRequiredDataPresent verifies that everything produced by
	// "setup" (files + Docker images) is available before the cluster
	// is created.
	CheckRequiredDataPresent(scenarioDir, version string) error

	// EventConfig returns the scaler-specific event configuration.
	EventConfig() ScalerEventConfig
}

// ExecArgs has the flag variables — bound by cobra, read once in execCmd.RunE, then
// passed explicitly to all callees so that no other function touches these globals.
type ExecArgs struct {
	Scaler        string
	SnapshotFile  string
	ConfigFile    string
	ScalerVersion string
	PricingFile   string
	SkipCleanup   bool
	WaitForCancel bool
}

// ScalerEventConfig describes the events a scaler emits and which ones
// indicate a pod has been deemed unschedulable.
type ScalerEventConfig struct {
	// Event source to match (e.g. "karpenter", "cluster-autoscaler")
	Source string
	// Event names to watch for (e.g. "FailedScheduling", "NodeCreated", "PodScheduled")
	WatchedEvents []string
	// Subset of WatchedEvents that mark a pod as unschedulable
	MarksPodUnschedulable []string
}

// KwokctlConfigTemplateParams stores all the parameters
// needed for the kwokctl configuration template.
type KwokctlConfigTemplateParams struct {
	HomeDir                 string
	ClusterName             string
	KubeSchedulerConfigPath string
	OutputPath              string
	ScenarioDirectory       string
	ImageTag                string
	PrometheusConfigPath    string
}

// monitorState groups the resources needed for metrics collection and
// event watching during a benchmark run.
type monitorState struct {
	metrics       map[string][]ContainerStats
	ec            *EventCollector
	wg            *sync.WaitGroup
	server        *http.Server
	cfg           *envconf.Config
	meta          *RunMetadata
	cancelStream  context.CancelFunc
	clusterName   string
	scenarioDir   string
	monitors      []DockerMonitor
	waitForCancel bool
}

// PrometheusConfigParams holds the parameters for the prometheus configuration template.
type prometheusConfigParams struct {
	HostIP         string
	ScrapeInterval string
	Port           int
}

// ---------------------------------------------------------------------------
// Docker
// ---------------------------------------------------------------------------

// DockerMonitor per docker container
type DockerMonitor struct {
	httpClient          *http.Client
	containerNamePrefix string
	containerID         string
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
		ThrottlingData struct {
			Periods          uint64 `json:"periods"`
			ThrottledPeriods uint64 `json:"throttled_periods"`
			ThrottledTime    uint64 `json:"throttled_time"`
		} `json:"throttling_data"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	PidsStats struct {
		Current uint32 `json:"current"`
	} `json:"pids_stats"`
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

// ScalingEvent represents a single event in the scaling timeline.
type ScalingEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Source    string    `json:"source"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace,omitempty"`
	Details   string    `json:"details,omitempty"`
	Region    string    `json:"region,omitempty"`
}

// TimingBreakdown captures the different durations during scaling.
type TimingBreakdown struct {
	FirstFailedScheduling time.Time `json:"firstFailedScheduling,omitzero"`
	FirstNodeCreated      time.Time `json:"firstNodeCreated,omitzero"`
	LastScaleInTime       time.Time `json:"lastScaleOutTime,omitzero"`
	LastScaleOutTime      time.Time `json:"lastScaleInTime,omitzero"`
	LastPodResolved       time.Time `json:"lastPodResolved,omitzero"`

	ReactionTime   string `json:"reactionTime"`
	ScaleOutTime   string `json:"scaleOutTime"`
	ScaleInTime    string `json:"scaleInTime"`
	SchedulingTime string `json:"schedulingTime"`
	TotalDuration  string `json:"totalDuration"`
}

type podSchedulingDuration struct {
	UID            string
	TimeToSchedule time.Duration `json:"timeToSchedule,inline"`
}

// EventCollector watches Kubernetes events, nodes, and pods to build
// a timeline for scaling/scheduling and tracks unschedulablePods.
type EventCollector struct {
	res  *resources.Resources
	done chan struct{}
	// unschedulablePods is a set consisting of pods that could not trigger a scale
	// up from the scaler
	unschedulablePods sets.Set[string]
	// The value is a slice since the same pod when deleted, is created with a different
	// UID, hence the durations track the UID and the scheduling information for each
	// lifetime of the pod.
	podSchedulingDurations map[string][]podSchedulingDuration
	timing                 TimingBreakdown
	eventConfig            ScalerEventConfig
	watchers               []*watcher.EventHandlerFuncs
	events                 []ScalingEvent
	// unscheduledCounter is initialised with the count of unscheduled non-daemonset
	// pods, the counter is updated for recreated pre-empted pods or pods that were
	// deleted due to node deletion.
	unscheduledCounter int
	// scheduledCount tracks pod scheduling, incremented when a 'PodScheduled'
	// event is raised.
	scheduledCount int
	mu             sync.Mutex
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

// PodMetrics represents the metrics of a pod at a point in time.
type PodMetrics struct {
	Timestamp  metav1.Time
	Containers []ContainerMetrics
}

// ContainerStats holds all resource metrics for a container at a point in time.
type ContainerStats struct {
	Timestamp           time.Time
	CPUMillicores       uint64
	MemoryMi            uint64
	MemoryRSSMi         uint64
	MemoryMaxUsageMi    uint64
	MemoryLimitMi       uint64
	CPUThrottledPeriods uint64
	CPUTotalPeriods     uint64
	CPUThrottledTimeNs  uint64
	PID                 uint32
}

// ContainerMetrics represents the resource usage of a single container.
type ContainerMetrics struct {
	Name  string
	Stats ContainerStats
}

// SchedulingLatency captures per-pod scheduling latency percentiles.
type SchedulingLatency struct {
	P50 string `json:"p50"`
	P90 string `json:"p90"`
	P99 string `json:"p99"`
	Max string `json:"max"`
}

// ScalingFailure represents a pod that could not be scheduled.
type ScalingFailure struct {
	PodName string `json:"podName"`
	Reason  string `json:"reason"`
	Details string `json:"details"`
}

// ClusterStats captures a point-in-time snapshot of cluster size.
type ClusterStats struct {
	NodeCount                   int `json:"nodeCount"`
	ScheduledPods               int `json:"scheduledPods"`
	UnscheduledNonDaemonSetPods int `json:"unscheduledNonDaemonSetPods"`
}

// ClusterState holds the cluster state before and after scaling.
type ClusterState struct {
	Before ClusterStats `json:"before"`
	After  ClusterStats `json:"after"`
}

// EventsSummary holds event count information.
type EventsSummary struct {
	CountByType map[string]int `json:"countByType"`
	TotalCount  int            `json:"totalCount"`
}

// InstanceDetails consists of information used to identify the
// scaled instances and the associated pricing/region data
type InstanceDetails struct {
	Region string
	Price  float64
	Count  int
}

// NodesSummary holds node scaling information.
type NodesSummary struct {
	InstanceTypes    map[string]InstanceDetails `json:"instanceTypes"`
	TotalCreated     int                        `json:"totalCreated"`
	TotalHourlyPrice float64                    `json:"totalHourlyPrice"`
}

// Summary holds the structured summary of the benchmark run.
type Summary struct {
	ScalingTimeline TimingBreakdown `json:"scalingTimeline"`
	Events          EventsSummary   `json:"events"`
	Nodes           NodesSummary    `json:"nodes"`
	Pods            PodsSummary     `json:"pods"`
	ClusterState    ClusterState    `json:"clusterState"`
}

// PodsSummary holds pod scheduling information.
type PodsSummary struct {
	SchedulingDurations map[string][]string `json:"schedulingDurations,omitempty"`
	SchedulingLatency   SchedulingLatency   `json:"schedulingLatency,omitzero"`
	Failures            []ScalingFailure    `json:"failures,omitempty"`
	UnschedulablePods   int                 `json:"unschedulablePods"`
}

// RunMetadata holds static information about a benchmark run known before execution.
type RunMetadata struct {
	StartTime        time.Time `json:"startTime"`
	EndTime          time.Time `json:"endTime"`
	TotalRunDuration string    `json:"totalRunDuration"`
	ScalerName       string    `json:"scalerName"`
	ScalerVersion    string    `json:"scalerVersion"`
	SnapshotFile     string    `json:"snapshotFile"`
	Summary          Summary   `json:"summary"`
}
