// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"log"
	"math"
	"slices"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/k8s/watcher"
)

// ScalingEvent represents a single event in the scaling timeline.
type ScalingEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Source    string    `json:"source"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace,omitempty"`
	Details   string    `json:"details,omitempty"`
}

// TimingBreakdown captures the different durations during scaling.
type TimingBreakdown struct {
	FirstFailedScheduling time.Time `json:"firstFailedScheduling,omitzero"`
	FirstNodeCreated      time.Time `json:"firstNodeCreated,omitzero"`
	LastPodResolved       time.Time `json:"lastPodResolved,omitzero"`
	ReactionTime          string    `json:"reactionTime"`
	SchedulingTime        string    `json:"schedulingTime"`
	TotalDuration         string    `json:"totalDuration"`
}

// EventCollector watches Kubernetes events, nodes, and pods to build
// a timeline and compute timing.
type EventCollector struct {
	// Setup
	res         *resources.Resources
	eventConfig ScalerEventConfig

	// Internal state
	mu       sync.Mutex
	watchers []*watcher.EventHandlerFuncs
	done     chan struct{}

	unscheduledCounter int
	scheduledCount     int
	unschedulablePods  sets.Set[string]

	// Collected data
	events             []ScalingEvent
	timing             TimingBreakdown
	podScheduleLatency map[string]time.Duration
}

// NewEventCollector creates an EventCollector that watches for scaling events.
func NewEventCollector(res *resources.Resources, unscheduledCount int, eventConfig ScalerEventConfig) *EventCollector {
	return &EventCollector{
		res:                res,
		unscheduledCounter: unscheduledCount,
		eventConfig:        eventConfig,
		done:               make(chan struct{}),
	}
}

// Start begins three watches: Events (FailedScheduling), Nodes (Added), Pods (Modified/Scheduled).
func (ec *EventCollector) Start(ctx context.Context) error {
	if ec.unscheduledCounter <= 0 {
		ec.finish()
		return nil
	}

	existingNodes := &corev1.NodeList{}
	if err := ec.res.List(ctx, existingNodes); err != nil {
		return err
	}
	existingNodeNames := make(map[string]bool, len(existingNodes.Items))
	for _, n := range existingNodes.Items {
		existingNodeNames[n.Name] = true
	}

	ec.unschedulablePods = sets.New[string]()
	ec.podScheduleLatency = make(map[string]time.Duration)

	if err := ec.watchEvents(ctx); err != nil {
		return err
	}
	if err := ec.watchNodes(ctx, existingNodeNames); err != nil {
		return err
	}
	if err := ec.watchPods(ctx); err != nil {
		return err
	}

	log.Printf("Watchers started: watching for %d pods to be scheduled\n", ec.unscheduledCounter)
	return nil
}

func (ec *EventCollector) watchEvents(ctx context.Context) error {
	ew := ec.res.Watch(&corev1.EventList{}, func(listOpts *metav1.ListOptions) {
		listOpts.ResourceVersion = "0"
	})
	ew.WithAddFunc(func(obj any) {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return
		}
		ec.mu.Lock()
		defer ec.mu.Unlock()

		// Use event.Source.Component if available, otherwise fall back to event.ReportingController
		source := event.Source.Component
		if source == "" {
			source = event.ReportingController
		}

		cfg := ec.eventConfig
		if event.Reason == "FailedScheduling" && (source == "default-scheduler" || source == "kube-scheduler") {
			if ec.timing.FirstFailedScheduling.IsZero() {
				ec.timing.FirstFailedScheduling = event.CreationTimestamp.Time
				ec.events = append(ec.events, ScalingEvent{
					Timestamp: event.CreationTimestamp.Time,
					Type:      event.Reason,
					Source:    source,
					Name:      event.InvolvedObject.Name,
					Namespace: event.InvolvedObject.Namespace,
					Details:   event.Message,
				})
			}
		} else if source == cfg.Source && slices.Contains(cfg.EventNames, event.Reason) {
			ec.events = append(ec.events, ScalingEvent{
				Timestamp: event.CreationTimestamp.Time,
				Type:      event.Reason,
				Source:    source,
				Name:      event.InvolvedObject.Name,
				Namespace: event.InvolvedObject.Namespace,
				Details:   event.Message,
			})
			if slices.Contains(cfg.MarksPodUnschedulable, event.Reason) {
				ec.podUnschedulable(event.InvolvedObject.Namespace + "/" + event.InvolvedObject.Name)
			}
		}
	})
	if err := ew.Start(ctx); err != nil {
		return err
	}
	ec.watchers = append(ec.watchers, ew)
	return nil
}

func (ec *EventCollector) watchNodes(ctx context.Context, existingNodeNames map[string]bool) error {
	nw := ec.res.Watch(&corev1.NodeList{}, func(listOpts *metav1.ListOptions) {
		listOpts.ResourceVersion = "0"
	})
	nw.WithAddFunc(func(obj any) {
		node, ok := obj.(*corev1.Node)
		if !ok {
			return
		}
		if existingNodeNames[node.Name] {
			return
		}
		ec.mu.Lock()
		defer ec.mu.Unlock()

		instanceType := node.Labels[corev1.LabelInstanceTypeStable]
		ec.events = append(ec.events, ScalingEvent{
			Timestamp: node.CreationTimestamp.Time,
			Type:      "NodeCreated",
			Source:    "node-watch",
			Name:      node.Name,
			Details:   instanceType,
		})

		if ec.timing.FirstNodeCreated.IsZero() {
			ec.timing.FirstNodeCreated = node.CreationTimestamp.Time
			ec.computeReactionTime()
		}
		NodesCreatedTotal.Inc()
	})
	if err := nw.Start(ctx); err != nil {
		return err
	}
	ec.watchers = append(ec.watchers, nw)
	return nil
}

func (ec *EventCollector) watchPods(ctx context.Context) error {
	pw := ec.res.Watch(&corev1.PodList{}, func(listOpts *metav1.ListOptions) {
		listOpts.ResourceVersion = "0"
	})
	pw.WithUpdateFunc(func(obj any) {
		pod, ok := obj.(*corev1.Pod)
		if !ok || pod.Spec.NodeName == "" {
			return
		}
		ec.mu.Lock()
		defer ec.mu.Unlock()
		ec.podScheduled(pod)
	})
	if err := pw.Start(ctx); err != nil {
		return err
	}
	ec.watchers = append(ec.watchers, pw)
	return nil
}

// Wait blocks until all unscheduled pods are scheduled or the context is cancelled.
func (ec *EventCollector) Wait(ctx context.Context) error {
	select {
	case <-ec.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop terminates all active watches.
func (ec *EventCollector) Stop() {
	for _, w := range ec.watchers {
		w.Stop()
	}
}

// Results returns the timeline events, timing breakdown, and enriched summary.
func (ec *EventCollector) Results() ([]ScalingEvent, TimingBreakdown, Summary) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	countByType := make(map[string]int)
	instanceTypes := make(map[string]int)
	var failures []ScalingFailure

	var timeline []ScalingEvent
	for _, e := range ec.events {
		countByType[e.Type]++
		if e.Type == "NodeCreated" && e.Details != "" {
			instanceTypes[e.Details]++
		}

		if e.Source == ec.eventConfig.Source && slices.Contains(ec.eventConfig.MarksPodUnschedulable, e.Type) {
			failures = append(failures, ScalingFailure{
				PodName: e.Name,
				Reason:  e.Type,
				Details: e.Details,
			})
		}
		timeline = append(timeline, e)
	}

	var schedulingDurations map[string]string
	if len(ec.podScheduleLatency) > 0 {
		schedulingDurations = make(map[string]string, len(ec.podScheduleLatency))
		for name, d := range ec.podScheduleLatency {
			schedulingDurations[name] = d.String()
		}
	}

	summary := Summary{
		Events: EventsSummary{
			TotalCount:  len(ec.events),
			CountByType: countByType,
		},
		Nodes: NodesSummary{
			TotalCreated:  countByType["NodeCreated"],
			InstanceTypes: instanceTypes,
		},
		Pods: PodsSummary{
			UnschedulablePods:   ec.unschedulablePods.Len(),
			SchedulingLatency:   ec.computeSchedulingLatency(),
			SchedulingDurations: schedulingDurations,
			Failures:            failures,
		},
	}

	return timeline, ec.timing, summary
}

// exitCriteriaMet checks if the number of unschedulable pods plus the number of scheduled pods
// meets or exceeds the total number of unscheduled pods we are waiting for
func (ec *EventCollector) exitCriteriaMet() bool {
	return ec.unschedulablePods.Len()+ec.scheduledCount >= ec.unscheduledCounter
}

func (ec *EventCollector) podUnschedulable(podName string) {
	if ec.unschedulablePods.Has(podName) {
		return
	}
	ec.unschedulablePods.Insert(podName)
	log.Printf("Pod %q marked unschedulable\n", podName)

	if ec.exitCriteriaMet() {
		ec.timing.LastPodResolved = time.Now()
		ec.computeSchedulingTime()
		ec.computeTotalDuration()
		ec.finish()
	}
}

func (ec *EventCollector) podScheduled(pod *corev1.Pod) {
	key := pod.Namespace + "/" + pod.Name
	if _, exists := ec.podScheduleLatency[key]; exists {
		return
	}

	scheduledTime := time.Now()
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
			scheduledTime = cond.LastTransitionTime.Time
			break
		}
	}

	ec.events = append(ec.events, ScalingEvent{
		Timestamp: scheduledTime,
		Type:      "PodScheduled",
		Source:    "pod-watch",
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Details:   pod.Spec.NodeName,
	})

	ec.podScheduleLatency[key] = scheduledTime.Sub(pod.CreationTimestamp.Time)

	ec.scheduledCount++
	PodsScheduledTotal.Inc()

	if ec.exitCriteriaMet() {
		ec.timing.LastPodResolved = scheduledTime
		ec.computeSchedulingTime()
		ec.computeTotalDuration()
		ec.finish()
	}
}

func (ec *EventCollector) finish() {
	select {
	case <-ec.done:
	default:
		close(ec.done)
	}
}

func (ec *EventCollector) computeSchedulingLatency() SchedulingLatency {
	if len(ec.podScheduleLatency) == 0 {
		return SchedulingLatency{}
	}

	durations := make([]time.Duration, 0, len(ec.podScheduleLatency))
	for _, d := range ec.podScheduleLatency {
		if d > 0 {
			durations = append(durations, d)
		}
	}
	if len(durations) == 0 {
		return SchedulingLatency{}
	}

	slices.Sort(durations)

	return SchedulingLatency{
		P50: percentile(durations, 50).String(),
		P90: percentile(durations, 90).String(),
		P99: percentile(durations, 99).String(),
		Max: durations[len(durations)-1].String(),
	}
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := max(int(math.Ceil(float64(p)/100.0*float64(len(sorted))))-1, 0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (ec *EventCollector) computeReactionTime() {
	if ec.timing.FirstFailedScheduling.IsZero() || ec.timing.FirstNodeCreated.IsZero() {
		return
	}
	duration := ec.timing.FirstNodeCreated.Sub(ec.timing.FirstFailedScheduling)
	ec.timing.ReactionTime = duration.String()
}

func (ec *EventCollector) computeSchedulingTime() {
	if ec.timing.FirstNodeCreated.IsZero() || ec.timing.LastPodResolved.IsZero() {
		return
	}
	duration := ec.timing.LastPodResolved.Sub(ec.timing.FirstNodeCreated)
	ec.timing.SchedulingTime = duration.String()
}

func (ec *EventCollector) computeTotalDuration() {
	if ec.timing.FirstFailedScheduling.IsZero() || ec.timing.LastPodResolved.IsZero() {
		return
	}
	duration := ec.timing.LastPodResolved.Sub(ec.timing.FirstFailedScheduling)
	ec.timing.TotalDuration = duration.String()
}
