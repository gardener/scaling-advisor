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
	"time"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	scrapeInterval = "1s"
	dockerHostIP   = "host.docker.internal"
)

// These container resource usage metrics are defined here and not captured via
// an external tool like 'cadvisor' since it needs priviledged access when running
// in a docker-compose project which 'kwokctl' docker runtime relies on.
// Without those permissions, the metrics scraped by 'cadvisor' don't have the
// proper container name labels added to them.

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

	// NodesDeletedTotal counts nodes scaled down by the scaler during the harness run.
	NodesDeletedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "scaling_nodes_deleted_total",
			Help: "Total number of nodes scaled down by the scaler",
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

func init() {
	prometheus.MustRegister(ContainerCPUUsage)
	prometheus.MustRegister(ContainerMemoryUsage)
	prometheus.MustRegister(ContainerMemoryLimit)
	prometheus.MustRegister(ContainerCPUThrottledPeriods)
	prometheus.MustRegister(ContainerCPUTotalPeriods)
	prometheus.MustRegister(ContainerCPUThrottledTime)
	prometheus.MustRegister(ContainerPIDs)
	prometheus.MustRegister(NodesCreatedTotal)
	prometheus.MustRegister(NodesDeletedTotal)
	prometheus.MustRegister(PodsScheduledTotal)
}

func writePrometheusConfig(destPath string, clusterName string, scalerName string, scalerPort int) error {
	params := prometheusConfigParams{
		HostIP:            dockerHostIP,
		Port:              benchutil.PrometheusPort,
		ScrapeInterval:    scrapeInterval,
		ClusterName:       clusterName,
		ScalerName:        scalerName,
		ScalerMetricsPort: scalerPort,
	}

	data, err := content.ReadFile("templates/prometheus-config.yaml")
	if err != nil {
		return fmt.Errorf("cannot read templates/prometheus-config.yaml: %w", err)
	}

	tmpl, err := template.New("prometheus-config.yaml").Parse(string(data))
	if err != nil {
		return fmt.Errorf("cannot parse prometheus-config.yaml template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return fmt.Errorf("cannot execute prometheus-config template: %w", err)
	}

	if err := os.WriteFile(destPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("cannot write prometheus config to %q: %w", destPath, err)
	}

	return nil
}

// ServeMetrics starts a prometheus metrics server and returns the server
// so it can be shut down gracefully.
func ServeMetrics(port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// G112 (CWE-400): Potential Slowloris Attack: kept it same as the one defined for http server started in the actual kube-apiserver.
		// See: https://github.com/kubernetes/kubernetes/blob/ad82c3d39f5e9f21e173ffeb8aa57953a0da4601/staging/src/k8s.io/apiserver/pkg/server/secure_serving.go#L172
		ReadHeaderTimeout: 32 * time.Second,
	}
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
	ContainerMemoryLimit.WithLabelValues(containerName).Set(float64(s.MemoryLimitMi))
	ContainerCPUThrottledPeriods.WithLabelValues(containerName).Set(float64(s.CPUThrottledPeriods))
	ContainerCPUTotalPeriods.WithLabelValues(containerName).Set(float64(s.CPUTotalPeriods))
	ContainerCPUThrottledTime.WithLabelValues(containerName).Set(float64(s.CPUThrottledTimeNs))
	ContainerPIDs.WithLabelValues(containerName).Set(float64(s.PID))
}

// ResetContainerMetrics zeroes all prometheus gauges for the given container.
func ResetContainerMetrics(containerName string) {
	ContainerCPUUsage.WithLabelValues(containerName).Set(0)
	ContainerMemoryUsage.WithLabelValues(containerName).Set(0)
	ContainerMemoryLimit.WithLabelValues(containerName).Set(0)
	ContainerCPUThrottledPeriods.WithLabelValues(containerName).Set(0)
	ContainerCPUTotalPeriods.WithLabelValues(containerName).Set(0)
	ContainerCPUThrottledTime.WithLabelValues(containerName).Set(0)
	ContainerPIDs.WithLabelValues(containerName).Set(0)
}
