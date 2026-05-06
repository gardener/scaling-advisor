// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodMetrics represents the metrics of a pod at a point in time.
type PodMetrics struct {
	Timestamp  metav1.Time        `json:"timestamp"`
	Containers []ContainerMetrics `json:"containers"`
}

// ContainerStats holds all resource metrics for a container.
type ContainerStats struct {
	CPUMillicores       int64  `json:"cpuMillicores"`
	MemoryMi            int64  `json:"memoryMi"`
	MemoryRSSMi         int64  `json:"memoryRSSMi"`
	MemoryMaxUsageMi    int64  `json:"memoryMaxUsageMi"`
	MemoryLimitMi       int64  `json:"memoryLimitMi"`
	CPUThrottledPeriods uint64 `json:"cpuThrottledPeriods"`
	CPUTotalPeriods     uint64 `json:"cpuTotalPeriods"`
	CPUThrottledTimeNs  uint64 `json:"cpuThrottledTimeNs"`
	PIDs                uint32 `json:"pids"`
}

// ContainerMetrics represents the resource usage of a single container.
type ContainerMetrics struct {
	Name  string         `json:"name"`
	Stats ContainerStats `json:"stats"`
}

// ClusterPodStats captures a point-in-time snapshot of cluster size.
type ClusterPodStats struct {
	NodeCount       int `json:"nodeCount"`
	ScheduledPods   int `json:"scheduledPods"`
	UnscheduledPods int `json:"unscheduledPods"`
}

// ClusterState holds the cluster state before and after scaling.
type ClusterState struct {
	Before ClusterPodStats `json:"before"`
	After  ClusterPodStats `json:"after"`
}

// RunMetadata holds static information about a benchmark run known before execution.
type RunMetadata struct {
	StartTime    time.Time       `json:"startTime"`
	ScalerName   string          `json:"scalerName"`
	ScalerVersion string         `json:"scalerVersion"`
	SnapshotFile string          `json:"snapshotFile"`
	ClusterState ClusterState    `json:"clusterState"`
	ScalingTime  TimingBreakdown `json:"scalingTime"`
}

// RunReport is the top-level structure written to the report file.
type RunReport struct {
	Metadata RunMetadata    `json:"metadata"`
	Metrics  []PodMetrics   `json:"metrics"`
	Events   []ScalingEvent `json:"events"`
}

// writeReport serializes the report to a JSON file at filePath.
func writeReport(filePath string, report RunReport) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("cannot create report file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("cannot encode report: %w", err)
	}
	return nil
}
