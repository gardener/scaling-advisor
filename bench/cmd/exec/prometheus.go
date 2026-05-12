// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ContainerCPUUsage tracks current CPU usage in millicores per container.
	ContainerCPUUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_cpu_usage_millicores",
			Help: "Current CPU usage in millicores",
		},
		[]string{"container_name"},
	)

	// ContainerMemoryUsage tracks current memory usage in megabytes per container.
	ContainerMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_memory_usage_megabytes",
			Help: "Current memory usage in megabytes",
		},
		[]string{"container_name"},
	)

	// ContainerMemoryRSS tracks current RSS memory in megabytes per container.
	ContainerMemoryRSS = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_memory_rss_megabytes",
			Help: "Current RSS memory in megabytes",
		},
		[]string{"container_name"},
	)

	// ContainerMemoryMaxUsage tracks peak memory usage in megabytes per container.
	ContainerMemoryMaxUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_memory_max_usage_megabytes",
			Help: "Peak memory usage in megabytes",
		},
		[]string{"container_name"},
	)

	// ContainerMemoryLimit tracks memory limit in megabytes per container.
	ContainerMemoryLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_memory_limit_megabytes",
			Help: "Memory limit in megabytes",
		},
		[]string{"container_name"},
	)

	// ContainerCPUThrottledPeriods tracks the number of throttled CPU periods per container.
	ContainerCPUThrottledPeriods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_cpu_throttled_periods",
			Help: "Number of CPU periods where the container was throttled",
		},
		[]string{"container_name"},
	)

	// ContainerCPUTotalPeriods tracks the total number of CPU scheduling periods per container.
	ContainerCPUTotalPeriods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_cpu_total_periods",
			Help: "Total number of CPU scheduling periods",
		},
		[]string{"container_name"},
	)

	// ContainerCPUThrottledTime tracks total CPU throttled time in nanoseconds per container.
	ContainerCPUThrottledTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_cpu_throttled_time_ns",
			Help: "Total CPU throttled time in nanoseconds",
		},
		[]string{"container_name"},
	)

	// ContainerPIDs tracks the current number of PIDs per container.
	ContainerPIDs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_pids_current",
			Help: "Current number of PIDs in the container",
		},
		[]string{"container_name"},
	)

	// NodesCreatedTotal counts new nodes created by the scaler during the benchmark.
	NodesCreatedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "scaling_nodes_created_total",
			Help: "Total number of new nodes created by the scaler",
		},
	)

	// PodsScheduledTotal counts previously-unscheduled pods that got scheduled during the benchmark.
	PodsScheduledTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "scaling_pods_scheduled_total",
			Help: "Total number of previously-unscheduled pods that got scheduled",
		},
	)
)

var prometheusScrapeInterval = "1s"

// PrometheusConfigParams holds the parameters for the prometheus configuration template.
type prometheusConfigParams struct {
	HostIP         string
	Port           int
	ScrapeInterval string
}

func writePrometheusConfig(port int) (string, error) {
	params := prometheusConfigParams{
		HostIP:         "host.docker.internal",
		Port:           port,
		ScrapeInterval: prometheusScrapeInterval,
	}

	data, err := content.ReadFile("templates/prometheus-config.yaml")
	if err != nil {
		return "", fmt.Errorf("cannot read templates/prometheus-config.yaml: %w", err)
	}

	tmpl, err := template.New("prometheus-config.yaml").Parse(string(data))
	if err != nil {
		return "", fmt.Errorf("cannot parse prometheus-config.yaml template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("cannot execute prometheus-config template: %w", err)
	}

	tempFile, err := os.CreateTemp("", "prometheus.yaml")
	if err != nil {
		return "", fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer tempFile.Close()

	if _, err := tempFile.Write(buf.Bytes()); err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("cannot write to temporary file: %w", err)
	}

	return tempFile.Name(), nil
}

func init() {
	prometheus.MustRegister(ContainerCPUUsage)
	prometheus.MustRegister(ContainerMemoryUsage)
	prometheus.MustRegister(ContainerMemoryRSS)
	prometheus.MustRegister(ContainerMemoryMaxUsage)
	prometheus.MustRegister(ContainerMemoryLimit)
	prometheus.MustRegister(ContainerCPUThrottledPeriods)
	prometheus.MustRegister(ContainerCPUTotalPeriods)
	prometheus.MustRegister(ContainerCPUThrottledTime)
	prometheus.MustRegister(ContainerPIDs)
	prometheus.MustRegister(NodesCreatedTotal)
	prometheus.MustRegister(PodsScheduledTotal)
}

// ServeMetrics starts a prometheus metrics server and returns the server
// so it can be shut down gracefully.
func ServeMetrics(port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{Addr: addr, Handler: mux}
	log.Printf("Serving metrics on %s\n", addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()
	return srv
}

// ---------------------------------------------------------------------------
// Utils
// ---------------------------------------------------------------------------

// SetContainerMetrics updates all prometheus gauges for the given container.
func SetContainerMetrics(containerName string, s ContainerStats) {
	ContainerCPUUsage.WithLabelValues(containerName).Set(float64(s.CPUMillicores))
	ContainerMemoryUsage.WithLabelValues(containerName).Set(float64(s.MemoryMi))
	ContainerMemoryRSS.WithLabelValues(containerName).Set(float64(s.MemoryRSSMi))
	ContainerMemoryMaxUsage.WithLabelValues(containerName).Set(float64(s.MemoryMaxUsageMi))
	ContainerMemoryLimit.WithLabelValues(containerName).Set(float64(s.MemoryLimitMi))
	ContainerCPUThrottledPeriods.WithLabelValues(containerName).Set(float64(s.CPUThrottledPeriods))
	ContainerCPUTotalPeriods.WithLabelValues(containerName).Set(float64(s.CPUTotalPeriods))
	ContainerCPUThrottledTime.WithLabelValues(containerName).Set(float64(s.CPUThrottledTimeNs))
	ContainerPIDs.WithLabelValues(containerName).Set(float64(s.PIDs))
}

// ResetContainerMetrics zeroes all prometheus gauges for the given container.
func ResetContainerMetrics(containerName string) {
	ContainerCPUUsage.WithLabelValues(containerName).Set(0)
	ContainerMemoryUsage.WithLabelValues(containerName).Set(0)
	ContainerMemoryRSS.WithLabelValues(containerName).Set(0)
	ContainerMemoryMaxUsage.WithLabelValues(containerName).Set(0)
	ContainerMemoryLimit.WithLabelValues(containerName).Set(0)
	ContainerCPUThrottledPeriods.WithLabelValues(containerName).Set(0)
	ContainerCPUTotalPeriods.WithLabelValues(containerName).Set(0)
	ContainerCPUThrottledTime.WithLabelValues(containerName).Set(0)
	ContainerPIDs.WithLabelValues(containerName).Set(0)
}
