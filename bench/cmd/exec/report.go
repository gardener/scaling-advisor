// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	reportFileName = "scaler-report.json"
	eventsFileName = "scaler-events.json"
)

// PodMetrics represents the metrics of a pod at a point in time.
type PodMetrics struct {
	Timestamp  metav1.Time
	Containers []ContainerMetrics
}

// ContainerStats holds all resource metrics for a container at a point in time.
type ContainerStats struct {
	Timestamp           time.Time
	CPUMillicores       int64
	MemoryMi            int64
	MemoryRSSMi         int64
	MemoryMaxUsageMi    int64
	MemoryLimitMi       int64
	CPUThrottledPeriods uint64
	CPUTotalPeriods     uint64
	CPUThrottledTimeNs  uint64
	PIDs                uint32
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
	NodeCount       int `json:"nodeCount"`
	ScheduledPods   int `json:"scheduledPods"`
	UnscheduledPods int `json:"unscheduledPods"`
}

// ClusterState holds the cluster state before and after scaling.
type ClusterState struct {
	Before ClusterStats `json:"before"`
	After  ClusterStats `json:"after"`
}

// EventsSummary holds event count information.
type EventsSummary struct {
	TotalCount  int            `json:"totalCount"`
	CountByType map[string]int `json:"countByType"`
}

type InstanceDetails struct {
	Price  float64
	Region string
	Count  int
}

// NodesSummary holds node scaling information.
type NodesSummary struct {
	TotalCreated  int                        `json:"totalCreated"`
	InstanceTypes map[string]InstanceDetails `json:"instanceTypes"`
}

// Summary holds the structured summary of the benchmark run.
type Summary struct {
	ScalingTimeline TimingBreakdown `json:"scalingTimeline"`
	ClusterState    ClusterState    `json:"clusterState"`
	Events          EventsSummary   `json:"events"`
	Nodes           NodesSummary    `json:"nodes"`
	Pods            PodsSummary     `json:"pods"`
}

// PodsSummary holds pod scheduling information.
type PodsSummary struct {
	UnschedulablePods   int               `json:"unschedulablePods"`
	SchedulingLatency   SchedulingLatency `json:"schedulingLatency,omitzero"`
	SchedulingDurations map[string]string `json:"schedulingDurations,omitempty"`
	Failures            []ScalingFailure  `json:"failures,omitempty"`
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

func writeReports(dir string, meta RunMetadata, events []ScalingEvent) {
	if err := writeJSON(path.Join(dir, reportFileName), meta); err != nil {
		log.Printf("Failed to write report: %v\n", err)
	}
	if err := writeJSON(path.Join(dir, eventsFileName), events); err != nil {
		log.Printf("Failed to write events: %v\n", err)
	}
	log.Printf("Wrote reports to %s\n", dir)
}

func writeJSON(filePath string, v any) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", filePath, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("cannot encode %s: %w", filePath, err)
	}
	return nil
}

var metricsCSVHeader = []string{
	"timestamp", "cpu_millicores", "memory_mi", "memory_rss_mi",
	"memory_max_usage_mi", "memory_limit_mi",
	"cpu_throttled_periods", "cpu_total_periods", "cpu_throttled_time_ns", "pids",
}

func writeMetricsCSV(dir string, metrics map[string][]ContainerStats) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create metrics directory: %v\n", err)
		return
	}
	for name, stats := range metrics {
		filePath := path.Join(dir, name+".csv")
		if err := writeContainerCSV(filePath, stats); err != nil {
			log.Printf("Failed to write metrics CSV for %s: %v\n", name, err)
		}
	}
	log.Printf("Wrote metrics CSVs to %s\n", dir)
}

func writeContainerCSV(filePath string, stats []ContainerStats) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(metricsCSVHeader); err != nil {
		return err
	}
	for _, s := range stats {
		record := []string{
			s.Timestamp.Format(time.RFC3339),
			strconv.FormatInt(s.CPUMillicores, 10),
			strconv.FormatInt(s.MemoryMi, 10),
			strconv.FormatInt(s.MemoryRSSMi, 10),
			strconv.FormatInt(s.MemoryMaxUsageMi, 10),
			strconv.FormatInt(s.MemoryLimitMi, 10),
			strconv.FormatUint(s.CPUThrottledPeriods, 10),
			strconv.FormatUint(s.CPUTotalPeriods, 10),
			strconv.FormatUint(s.CPUThrottledTimeNs, 10),
			strconv.FormatUint(uint64(s.PIDs), 10),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
