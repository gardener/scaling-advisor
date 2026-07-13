// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	reportFileName = "scaler-report.json"
	eventsFileName = "scaler-events.json"
)

// newMonitor creates a monitorState by discovering Docker containers and
// preparing the metrics infrastructure. Call start() to begin streaming.
func newMonitor(ctx context.Context, cfg *envconf.Config, meta *RunMetadata, clusterName, scenarioDir string) (*monitorState, error) {
	containersToMonitor := []string{
		meta.ScalerName, "kube-apiserver", "etcd", "kube-scheduler", "kwok-controller",
	}

	var monitors []DockerMonitor
	for _, name := range containersToMonitor {
		m := NewDockerMonitor(clusterName + "-" + name)
		if err := m.WaitForReady(ctx); err != nil {
			if name == meta.ScalerName {
				return nil, fmt.Errorf("scaler container %q did not become ready: %w", name, err)
			}
			log.Printf("Warning: component %q not found, skipping metrics collection", name)
			continue
		}
		monitors = append(monitors, m)
	}

	return &monitorState{
		monitors:    monitors,
		metrics:     make(map[string][]ContainerStats),
		cfg:         cfg,
		meta:        meta,
		clusterName: clusterName,
		scenarioDir: scenarioDir,
	}, nil
}

// start begins Docker stats streaming, serves Prometheus metrics, and
// starts the EventCollector. It should be called after newMonitor.
func (mon *monitorState) start(ctx context.Context, eventConfig ScalerEventConfig, dsPods sets.Set[string]) error {
	mon.server = ServeMetrics(benchutil.PrometheusPort)

	streamCtx, cancelStream := context.WithCancel(ctx)
	mon.cancelStream = cancelStream

	metricsChan := make(chan PodMetrics, 100)

	var wg sync.WaitGroup
	wg.Go(func() {
		for m := range metricsChan {
			ts := m.Timestamp.UTC()
			for _, container := range m.Containers {
				stats := container.Stats
				stats.Timestamp = ts
				mon.metrics[container.Name] = append(mon.metrics[container.Name], stats)
				SetContainerMetrics(container.Name, stats)
			}
		}
	})
	mon.wg = &wg

	var producerWg sync.WaitGroup
	producerWg.Add(len(mon.monitors))
	for _, m := range mon.monitors {
		go func() {
			defer producerWg.Done()
			if err := m.StreamMetrics(streamCtx, metricsChan); err != nil {
				log.Printf("Error collecting metrics for %s: %v", m.containerNamePrefix, err)
			}
		}()
	}
	go func() {
		producerWg.Wait()
		close(metricsChan)
	}()

	log.Println("Starting event collection and measuring scaling timeline...")
	mon.ec = NewEventCollector(mon.cfg.Client().Resources(), eventConfig, dsPods)
	if err := mon.ec.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event collector: %w", err)
	}
	return nil
}

// stop waits for scaling to complete, then writes the final report
// and shuts down the metrics server.
func (mon *monitorState) stop(pricingData pricingapi.InstancePricingAccess) {
	for _, m := range mon.monitors {
		ResetContainerMetrics(m.containerNamePrefix)
	}

	events, timing, eventsSummary := mon.ec.Results(pricingData)
	mon.ec.Stop()      // stop event watches
	mon.cancelStream() // stop all metric streams
	mon.meta.EndTime = time.Now().UTC()
	log.Println("Scaling complete")
	fmt.Printf(
		"Reaction time: %s, ScaleIn time: %s, ScaleOut time: %s, Scheduling time: %s\n",
		cmp.Or(timing.ReactionTime, "N/A"), cmp.Or(timing.ScaleInTime, "N/A"),
		cmp.Or(timing.ScaleOutTime, "N/A"), cmp.Or(timing.SchedulingTime, "N/A"),
	)

	mon.clusterStateAfter(context.Background())

	mon.wg.Wait()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mon.server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Metrics server shutdown error: %v", err)
	}

	eventsSummary.ScalingTimeline = timing
	eventsSummary.ClusterState = mon.meta.Summary.ClusterState
	mon.meta.Summary = eventsSummary
	mon.meta.TotalRunDuration = mon.meta.EndTime.Sub(mon.meta.StartTime).String()
	fmt.Printf("Total benchmarking run time: %s\n", mon.meta.TotalRunDuration)

	logsDir := path.Join(mon.scenarioDir, "out", "kwok-"+mon.clusterName)
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		log.Printf("Failed to create logs directory: %v\n", err)
	} else {
		writeReports(logsDir, *mon.meta, events)
	}
}

func (mon *monitorState) clusterStateAfter(ctx context.Context) {
	nodes := &corev1.NodeList{}
	if err := mon.cfg.Client().Resources().List(ctx, nodes); err == nil {
		mon.meta.Summary.ClusterState.After.NodeCount = len(nodes.Items)
	}

	pods := &corev1.PodList{}
	if err := mon.cfg.Client().Resources().List(ctx, pods); err == nil {
		for _, pod := range pods.Items {
			if pod.Spec.NodeName == "" {
				if !isOwner(pod.GetOwnerReferences(), benchutil.OwnerDaemonSet) {
					mon.meta.Summary.ClusterState.After.UnscheduledNonDaemonSetPods++
				}
			} else {
				mon.meta.Summary.ClusterState.After.ScheduledPods++
			}
		}
	}
}

func writeReports(dir string, meta RunMetadata, events []ScalingEvent) {
	if err := writeJSON(path.Join(dir, reportFileName), meta); err != nil {
		log.Printf("Failed to write report: %v\n", err)
	}
	if err := writeJSON(path.Join(dir, eventsFileName), events); err != nil {
		log.Printf("Failed to write events: %v\n", err)
	}
}

func writeJSON(filePath string, v any) error {
	f, err := os.Create(filepath.Clean(filePath))
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
