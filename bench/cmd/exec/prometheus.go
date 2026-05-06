// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ScalerCPUUsage registers the CPU usage of the scaler
	ScalerCPUUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_cpu_usage_millicores",
			Help: "Current CPU usage of the scaler in millicores",
		},
		[]string{"container_name"},
	)

	// ScalerMemoryUsage registers the Memory usage of the scaler
	ScalerMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_memory_usage_megabytes",
			Help: "Current memory usage of the scaler in megabytes",
		},
		[]string{"container_name"},
	)

	ScalerMemoryRSS = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_memory_rss_megabytes",
			Help: "Current RSS memory of the scaler in megabytes",
		},
		[]string{"container_name"},
	)

	ScalerMemoryMaxUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_memory_max_usage_megabytes",
			Help: "Peak memory usage of the scaler in megabytes",
		},
		[]string{"container_name"},
	)

	ScalerMemoryLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_memory_limit_megabytes",
			Help: "Memory limit of the scaler container in megabytes",
		},
		[]string{"container_name"},
	)

	ScalerCPUThrottledPeriods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_cpu_throttled_periods",
			Help: "Number of CPU periods where the scaler was throttled",
		},
		[]string{"container_name"},
	)

	ScalerCPUTotalPeriods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_cpu_total_periods",
			Help: "Total number of CPU scheduling periods",
		},
		[]string{"container_name"},
	)

	ScalerCPUThrottledTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_cpu_throttled_time_ns",
			Help: "Total CPU throttled time in nanoseconds",
		},
		[]string{"container_name"},
	)

	ScalerPIDs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "scaler_pids_current",
			Help: "Current number of PIDs in the scaler container",
		},
		[]string{"container_name"},
	)

	EventsFailedSchedulingTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "scaling_events_failed_scheduling_total",
			Help: "Total number of FailedScheduling events observed",
		},
	)

	NodesCreatedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "scaling_nodes_created_total",
			Help: "Total number of new nodes created by the scaler",
		},
	)

	PodsScheduledTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "scaling_pods_scheduled_total",
			Help: "Total number of previously-unscheduled pods that got scheduled",
		},
	)

	ScalingDecisionLatencySeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "scaling_decision_latency_seconds",
			Help: "Time from first FailedScheduling event to first node creation",
		},
	)

	ScalingSchedulingLatencySeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "scaling_scheduling_latency_seconds",
			Help: "Time from first node creation to last pod scheduled",
		},
	)

	ScalingTotalLatencySeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "scaling_total_latency_seconds",
			Help: "Time from first FailedScheduling to last pod scheduled",
		},
	)

	PodsScheduledProgress = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "scaling_pods_scheduled_progress",
			Help: "Current count of unscheduled pods that have been scheduled",
		},
	)
)

var PrometheusScrapeInterval = "1s"

type PrometheusConfigParams struct {
	HostIP         string
	Port           int
	ScrapeInterval string
}

func writePrometheusConfig(port int) (string, error) {
	params := PrometheusConfigParams{
		HostIP:         "host.docker.internal",
		Port:           port,
		ScrapeInterval: PrometheusScrapeInterval,
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
	prometheus.MustRegister(ScalerCPUUsage)
	prometheus.MustRegister(ScalerMemoryUsage)
	prometheus.MustRegister(ScalerMemoryRSS)
	prometheus.MustRegister(ScalerMemoryMaxUsage)
	prometheus.MustRegister(ScalerMemoryLimit)
	prometheus.MustRegister(ScalerCPUThrottledPeriods)
	prometheus.MustRegister(ScalerCPUTotalPeriods)
	prometheus.MustRegister(ScalerCPUThrottledTime)
	prometheus.MustRegister(ScalerPIDs)
	prometheus.MustRegister(EventsFailedSchedulingTotal)
	prometheus.MustRegister(NodesCreatedTotal)
	prometheus.MustRegister(PodsScheduledTotal)
	prometheus.MustRegister(ScalingDecisionLatencySeconds)
	prometheus.MustRegister(ScalingSchedulingLatencySeconds)
	prometheus.MustRegister(ScalingTotalLatencySeconds)
	prometheus.MustRegister(PodsScheduledProgress)
}

// ServeMetrics starts a prometheus metrics server
func ServeMetrics(port int) error {
	http.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Serving metrics on %s\n", addr)
	return http.ListenAndServe(addr, nil)
}
