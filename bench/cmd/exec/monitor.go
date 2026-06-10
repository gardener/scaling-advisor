// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"sync"
	"time"

	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
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
func (mon *monitorState) start(ctx context.Context, eventConfig ScalerEventConfig) error {
	mon.server = ServeMetrics(prometheusPort)

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
	mon.ec = NewEventCollector(mon.cfg.Client().Resources(), mon.meta.Summary.ClusterState.Before.UnscheduledNonDaemonSetPods, eventConfig)
	if err := mon.ec.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event collector: %w", err)
	}
	return nil
}

// stop waits for scaling to complete, then writes the final report
// and shuts down the metrics server.
func (mon *monitorState) stop(ctx context.Context, pricingData pricingapi.InstancePricingAccess) {
	err := mon.ec.Wait(ctx)
	if err != nil {
		log.Printf("Event collector wait interrupted: %v", err)
	}

	if mon.waitForCancel {
		for _, m := range mon.monitors {
			ResetContainerMetrics(m.containerNamePrefix)
		}
		log.Println("Waiting for Ctrl+C...")
		<-ctx.Done()
	}

	events, timing, eventsSummary := mon.ec.Results(pricingData)
	mon.ec.Stop()      // stop event watches
	mon.cancelStream() // stop all metric streams
	mon.meta.EndTime = time.Now().UTC()
	log.Printf("Scaling complete. Total time: %s, Reaction time: %s, Scheduling time: %s\n",
		cmp.Or(timing.TotalDuration, "N/A"), cmp.Or(timing.ReactionTime, "N/A"), cmp.Or(timing.SchedulingTime, "N/A"))

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
	log.Printf("Total benchmarking run time: %s\n", mon.meta.TotalRunDuration)

	logsDir := path.Join(mon.scenarioDir, "logs", "kwok-"+mon.clusterName)
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("Failed to create logs directory: %v\n", err)
	} else {
		writeReports(logsDir, *mon.meta, events)
		writeMetricsCSV(path.Join(logsDir, "metrics"), mon.metrics)
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
				if !isOwner(pod.GetOwnerReferences(), "Daemonset") {
					mon.meta.Summary.ClusterState.After.UnscheduledNonDaemonSetPods++
				}
			} else {
				mon.meta.Summary.ClusterState.After.ScheduledPods++
			}
		}
	}
}
