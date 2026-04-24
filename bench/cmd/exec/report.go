// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodMetrics represents the metrics of a pod at a point in time.
type PodMetrics struct {
	Timestamp  metav1.Time        `json:"timestamp"`
	Window     metav1.Duration    `json:"window"`
	Containers []ContainerMetrics `json:"containers"`
}

// ContainerMetrics represents the resource usage of a single container.
type ContainerMetrics struct {
	Name  string              `json:"name"`
	Usage corev1.ResourceList `json:"usage"`
}

// ClusterState captures a point-in-time snapshot of cluster size.
type ClusterState struct {
	NodeCount       int `json:"nodeCount"`
	ScheduledPods   int `json:"scheduledPods"`
	UnscheduledPods int `json:"unscheduledPods"`
}

// RunMetadata holds static information about a benchmark run known before execution.
type RunMetadata struct {
	StartTime              time.Time    `json:"startTime"`
	ScalerName             string       `json:"scalerName"`
	ScalerVersion          string       `json:"scalerVersion"`
	SnapshotFile           string       `json:"snapshotFile"`
	Before                 ClusterState `json:"before"`
	After                  ClusterState `json:"after"`
	RecommendationDuration string       `json:"recommendationDuration"`
}

// RunReport is the top-level structure written to the report file.
type RunReport struct {
	Metadata RunMetadata  `json:"metadata"`
	Metrics  []PodMetrics `json:"metrics"`
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
