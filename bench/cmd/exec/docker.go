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
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// dockerSocketPath returns the Docker socket path by checking DOCKER_HOST,
// then common socket locations.
func dockerSocketPath() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		if after, ok := strings.CutPrefix(host, "unix://"); ok {
			return after
		}
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/var/run/docker.sock",
		filepath.Join(home, ".docker", "run", "docker.sock"),
		filepath.Join(home, ".colima", "default", "docker.sock"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "/var/run/docker.sock"
}

// NewDockerMonitor creates a new DockerMonitor
func NewDockerMonitor(containerNamePrefix string) DockerMonitor {
	sockPath := dockerSocketPath()
	return DockerMonitor{
		containerNamePrefix: containerNamePrefix,
		httpClient:          newDialHTTPClient(sockPath),
	}
}

func newDialHTTPClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{Transport: transport}
}

// WaitForReady waits for the container to be running, polling every 500ms with a 20s timeout.
func (m *DockerMonitor) WaitForReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if id, err := m.findContainerIDByPrefix(ctx, m.containerNamePrefix); err == nil && id != "" {
		m.containerID = id
		// log.Printf("Found container: %s\n", m.containerNamePrefix)
		return nil
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for container %q", m.containerNamePrefix)
		case <-ticker.C:
			id, err := m.findContainerIDByPrefix(ctx, m.containerNamePrefix)
			if err != nil {
				continue
			}
			if id != "" {
				m.containerID = id
				log.Printf("Found container: %s\n", m.containerNamePrefix)
				return nil
			}
		}
	}
}

// StreamMetrics opens a streaming connection to Docker stats and sends metrics to the channel.
func (m *DockerMonitor) StreamMetrics(ctx context.Context, ch chan<- PodMetrics) error {
	if m.containerID == "" {
		return fmt.Errorf("container ID not set")
	}

	// log.Printf("Streaming metrics for container %s\n", m.containerNamePrefix)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://unix/containers/%s/stats?stream=true", m.containerID), nil)
	if err != nil {
		return fmt.Errorf("cannot create stats request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("docker stats stream failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker stats error: %s", strings.TrimSpace(string(body)))
	}

	dec := json.NewDecoder(resp.Body)
	first := true
	for {
		var stats dockerStats
		if err := dec.Decode(&stats); err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil || errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("error reading stats stream: %w", err)
		}

		// Skip the first stats update since it may contain zeroed values.
		if first {
			first = false
			continue
		}

		metric := m.parseStats(&stats)
		select {
		case ch <- *metric:
		case <-ctx.Done():
			return nil
		}
	}
}

func (m *DockerMonitor) parseStats(stats *dockerStats) *PodMetrics {
	var cpuMilli uint64
	if stats.PreCPUStats.SystemCPUUsage > 0 && stats.CPUStats.SystemCPUUsage > 0 {
		cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage)
		onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
		if systemDelta > 0 && onlineCPUs > 0 {
			percent := (cpuDelta / systemDelta) * onlineCPUs * 100.0
			cpuMilli = uint64(percent * 10.0)
		}
	}

	return &PodMetrics{
		Timestamp: metav1.NewTime(time.Now().UTC()),
		Containers: []ContainerMetrics{
			{
				Name: m.containerNamePrefix,
				Stats: ContainerStats{
					CPUMillicores:       cpuMilli,
					MemoryMi:            stats.MemoryStats.Usage / (1024 * 1024),
					MemoryLimitMi:       stats.MemoryStats.Limit / (1024 * 1024),
					CPUThrottledPeriods: stats.CPUStats.ThrottlingData.ThrottledPeriods,
					CPUTotalPeriods:     stats.CPUStats.ThrottlingData.Periods,
					CPUThrottledTimeNs:  stats.CPUStats.ThrottlingData.ThrottledTime,
					PID:                 stats.PidsStats.Current,
				},
			},
		},
	}
}

func (m *DockerMonitor) findContainerIDByPrefix(ctx context.Context, prefix string) (string, error) {
	filters := map[string][]string{"name": {prefix}}
	filtJSON, err := json.Marshal(filters)
	if err != nil {
		return "", fmt.Errorf("cannot marshal docker filters: %w", err)
	}
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

	var list []struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return "", err
	}
	if len(list) > 0 {
		return list[0].ID, nil
	}
	return "", nil
}
