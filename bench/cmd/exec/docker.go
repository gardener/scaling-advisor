// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// DockerMonitor per docker container
type DockerMonitor struct {
	containerNamePrefix string
	containerID         string
	interval            time.Duration
	httpClient          *http.Client
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
	} `json:"memory_stats"`
}

var (
	defaultDockerSocket = "/var/run/docker.sock"
)

// NewDockerMonitor creates a new DockerMonitor
func NewDockerMonitor(containerNamePrefix string, interval time.Duration) *DockerMonitor {
	return &DockerMonitor{
		containerNamePrefix: containerNamePrefix,
		interval:            interval,
		httpClient:          newDialHTTPClient(defaultDockerSocket),
	}
}

func newDialHTTPClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

// WaitForReady waits for the container to be ready
func (m *DockerMonitor) WaitForReady(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			id, err := m.findContainerIDByPrefix(ctx, m.containerNamePrefix)
			if err != nil {
				continue
			}
			if id != "" {
				m.containerID = id
				fmt.Printf("Found container: %s (id: %s)\n", m.containerNamePrefix, m.containerID)
				return nil
			}
		}
	}
}

// StreamMetrics collects metrics for the monitored container and sends them to the channel
func (m *DockerMonitor) StreamMetrics(ctx context.Context, ch chan<- PodMetrics) error {
	fmt.Printf("Collecting metrics for container prefix %s (id: %s)\n", m.containerNamePrefix, m.containerID)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	// Take an immediate sample so short runs still capture data
	if first, err := m.getDockerMetrics(ctx); err == nil {
		select {
		case ch <- *first:
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		fmt.Printf("Error getting initial metrics for container %s: %v\n", m.containerID, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			metric, err := m.getDockerMetrics(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || ctx.Err() != nil {
					return nil
				}
				fmt.Printf("Error getting metrics for container %s: %v\n", m.containerID, err)
				continue
			}
			select {
			case ch <- *metric:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (m *DockerMonitor) getDockerMetrics(ctx context.Context) (*PodMetrics, error) {
	if m.containerID == "" {
		return nil, fmt.Errorf("container ID not set")
	}

	stats, err := m.fetchContainerStats(ctx, m.containerID)
	if err != nil {
		return nil, err
	}

	var cpuMilli int64
	if stats.PreCPUStats.SystemCPUUsage > 0 && stats.CPUStats.SystemCPUUsage > 0 {
		cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage)
		onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
		if systemDelta > 0 && onlineCPUs > 0 {
			percent := (cpuDelta / systemDelta) * onlineCPUs * 100.0
			cpuMilli = int64(percent * 10.0)
		}
	}
	cpuQuantity := resource.NewMilliQuantity(cpuMilli, resource.DecimalSI)

	memUsage := int64(stats.MemoryStats.Usage)
	memQuantity := resource.NewQuantity(memUsage, resource.BinarySI)

	metric := &PodMetrics{
		Timestamp: metav1.NewTime(time.Now()),
		Window:    metav1.Duration{Duration: m.interval},
		Containers: []ContainerMetrics{
			{
				Name: m.containerNamePrefix,
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:    *cpuQuantity,
					corev1.ResourceMemory: *memQuantity,
				},
			},
		},
	}
	return metric, nil
}

func (m *DockerMonitor) findContainerIDByPrefix(ctx context.Context, prefix string) (string, error) {
	filters := map[string][]string{"name": {prefix}}
	filtJSON, _ := json.Marshal(filters)
	q := url.Values{}
	q.Set("filters", string(filtJSON))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/containers/json?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("docker API error: %s", strings.TrimSpace(string(body)))
	}

	type Container struct {
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
	}
	var list []Container
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return "", err
	}
	for _, c := range list {
		for _, n := range c.Names {
			if strings.Contains(n, prefix) {
				return c.ID, nil
			}
		}
	}
	return "", nil
}

func (m *DockerMonitor) fetchContainerStats(ctx context.Context, id string) (*dockerStats, error) {
	// stats endpoint may stream; with stream=false returns a single JSON and conn closes
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://unix/containers/%s/stats?stream=false", id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker stats error: %s", strings.TrimSpace(string(body)))
	}
	var stats dockerStats
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// MeasureRecommendationTime measures the time until a recommendation is produced.
// The condition function should return true when the recommendation is ready.
func MeasureRecommendationTime(ctx context.Context, start time.Time, condition func() (bool, error)) (time.Duration, error) {
	ticker := time.NewTicker(1 * time.Second) // Check every second
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ticker.C:
			ready, err := condition()
			if err != nil {
				return 0, err
			}
			if ready {
				return time.Since(start), nil
			}
		}
	}
}

// Wait for FailedScheduling event to appear
func getFailedSchedulingEventTime(ctx context.Context, cfg *envconf.Config) (time.Time, error) {
	for i := 0; i < 30; i++ {
		eventList := &corev1.EventList{}
		if err := cfg.Client().Resources().List(ctx, eventList); err != nil {
			return time.Time{}, err
		}

		for _, event := range eventList.Items {
			if event.Reason == "FailedScheduling" {
				eventTime := event.CreationTimestamp.Time
				log.Printf("Found FailedScheduling event at %v\n", eventTime)
				return eventTime, nil
			}
		}

		if i < 29 {
			time.Sleep(1 * time.Second)
		}
	}

	return time.Time{}, fmt.Errorf("no FailedScheduling event found after 30 attempts")
}
