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
)

type PrometheusConfigParams struct {
	HostIP string
	Port   int
}

func writePrometheusConfig(port int) (string, error) {
	params := PrometheusConfigParams{
		HostIP: "host.docker.internal",
		Port:   port,
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
}

// ServeMetrics starts a prometheus metrics server
func ServeMetrics(port int) error {
	http.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Serving metrics on %s\n", addr)
	return http.ListenAndServe(addr, nil)
}
