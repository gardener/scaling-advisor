// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"log"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/k8s/watcher"
)

// ScalingEvent represents a single event in the scaling timeline.
type ScalingEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace,omitempty"`
	Details   string    `json:"details,omitempty"`
}

// TimingBreakdown captures the different durations during scaling.
type TimingBreakdown struct {
	FirstFailedScheduling time.Time `json:"firstFailedScheduling,omitzero"`
	FirstNodeCreated      time.Time `json:"firstNodeCreated,omitzero"`
	LastPodScheduled      time.Time `json:"lastPodScheduled,omitzero"`
	ReactionTime          string    `json:"reactionTime"`
	SchedulingTime        string    `json:"schedulingTime"`
	TotalDuration         string    `json:"totalDuration"`
}

// EventCollector watches Kubernetes events, nodes, and pods to build
// a timeline and compute timing.
type EventCollector struct {
	res              *resources.Resources
	unscheduledCount int

	mu     sync.Mutex
	events []ScalingEvent
	timing TimingBreakdown

	scheduledCount    int
	remaining         int
	scheduledPods     map[string]bool
	unschedulablePods map[string]bool

	lastEventTime time.Time

	done     chan struct{}
	doneOnce sync.Once

	watchers []*watcher.EventHandlerFuncs
}

// NewEventCollector creates an EventCollector that watches for scaling events.
func NewEventCollector(res *resources.Resources, unscheduledCount int) *EventCollector {
	return &EventCollector{
		res:              res,
		unscheduledCount: unscheduledCount,
		done:             make(chan struct{}),
	}
}

// Start begins three watches: Events (FailedScheduling), Nodes (Added), Pods (Modified/Scheduled).
func (ec *EventCollector) Start(ctx context.Context) error {
	if ec.unscheduledCount <= 0 {
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

	existingPods := &corev1.PodList{}
	if err := ec.res.List(ctx, existingPods); err != nil {
		return err
	}
	ec.scheduledPods = make(map[string]bool)
	ec.unschedulablePods = make(map[string]bool)
	ec.remaining = ec.unscheduledCount

	eventList := &corev1.EventList{}
	if err := ec.res.List(ctx, eventList); err != nil {
		return err
	}

	if err := ec.watchEvents(ctx, eventList.ResourceVersion); err != nil {
		return err
	}
	if err := ec.watchNodes(ctx, existingNodes.ResourceVersion, existingNodeNames); err != nil {
		return err
	}
	if err := ec.watchPods(ctx, existingPods.ResourceVersion); err != nil {
		return err
	}

	log.Printf("Watchers started: watching for %d pods to be scheduled\n", ec.remaining)
	return nil
}

func (ec *EventCollector) watchEvents(ctx context.Context, resourceVersion string) error {
	ew := ec.res.Watch(&corev1.EventList{}, func(lo *metav1.ListOptions) {
		lo.ResourceVersion = resourceVersion
	})
	ew.WithAddFunc(func(obj any) {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return
		}
		ec.mu.Lock()
		defer ec.mu.Unlock()

		ec.lastEventTime = time.Now()

		if event.Reason == "FailedScheduling" {
			if ec.timing.FirstFailedScheduling.IsZero() {
				ec.timing.FirstFailedScheduling = event.CreationTimestamp.Time
				ec.events = append(ec.events, ScalingEvent{
					Timestamp: event.CreationTimestamp.Time,
					Type:      event.Reason,
					Name:      event.InvolvedObject.Name,
					Namespace: event.InvolvedObject.Namespace,
					Details:   event.Message,
				})
			}
			EventsFailedSchedulingTotal.Inc()
		} else if event.Reason != "Scheduled" {
			ec.events = append(ec.events, ScalingEvent{
				Timestamp: event.CreationTimestamp.Time,
				Type:      event.Reason,
				Name:      event.InvolvedObject.Name,
				Namespace: event.InvolvedObject.Namespace,
				Details:   event.Message,
			})
		}

		if event.Reason == "NotTriggerScaleUp" {
			ec.podUnschedulable(event.InvolvedObject.Name)
		}
	})
	if err := ew.Start(ctx); err != nil {
		return err
	}
	ec.watchers = append(ec.watchers, ew)
	return nil
}

func (ec *EventCollector) watchNodes(ctx context.Context, resourceVersion string, existingNodeNames map[string]bool) error {
	nw := ec.res.Watch(&corev1.NodeList{}, func(lo *metav1.ListOptions) {
		lo.ResourceVersion = resourceVersion
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

func (ec *EventCollector) watchPods(ctx context.Context, resourceVersion string) error {
	pw := ec.res.Watch(&corev1.PodList{}, func(lo *metav1.ListOptions) {
		lo.ResourceVersion = resourceVersion
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

// WaitForScalerEvents blocks until no new events have been received for the
// given quiet duration, or the context is cancelled. Call this after Wait()
// to let late-arriving events (e.g. TriggeredScaleUp) trickle in.
func (ec *EventCollector) WaitForScalerEvents(ctx context.Context, quiet time.Duration) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ec.mu.Lock()
			last := ec.lastEventTime
			ec.mu.Unlock()
			if !last.IsZero() && time.Since(last) >= quiet {
				return
			}
		}
	}
}

// Results returns the collected events and computed timing breakdown.
func (ec *EventCollector) Results() ([]ScalingEvent, TimingBreakdown, EventsSummary) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	events := make([]ScalingEvent, len(ec.events))
	copy(events, ec.events)

	meta := EventsSummary{
		TotalCount:        len(events),
		CountByType:       make(map[string]int),
		InstanceTypes:     make(map[string]int),
		UnschedulablePods: len(ec.unschedulablePods),
	}
	for _, e := range events {
		meta.CountByType[e.Type]++
		if e.Type == "NodeCreated" && e.Details != "" {
			meta.InstanceTypes[e.Details]++
		}
		if meta.FirstEventTime.IsZero() || e.Timestamp.Before(meta.FirstEventTime) {
			meta.FirstEventTime = e.Timestamp
		}
		if e.Timestamp.After(meta.LastEventTime) {
			meta.LastEventTime = e.Timestamp
		}
	}

	return events, ec.timing, meta
}

func (ec *EventCollector) finish() {
	ec.doneOnce.Do(func() {
		if ec.lastEventTime.IsZero() {
			ec.lastEventTime = time.Now()
		}
		close(ec.done)
	})
}

func (ec *EventCollector) podUnschedulable(podName string) {
	if ec.unschedulablePods[podName] {
		return
	}
	ec.unschedulablePods[podName] = true
	ec.remaining--
	log.Printf("Pod %q marked unschedulable (remaining: %d)\n", podName, ec.remaining)

	if ec.scheduledCount >= ec.remaining {
		ec.timing.LastPodScheduled = time.Now()
		ec.computeSchedulingTime()
		ec.computeTotalDuration()
		ec.finish()
	}
}

func (ec *EventCollector) podScheduled(pod *corev1.Pod) {
	uid := string(pod.UID)
	if ec.scheduledPods[uid] {
		return
	}
	ec.scheduledPods[uid] = true

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
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Details:   pod.Spec.NodeName,
	})

	ec.scheduledCount++
	PodsScheduledTotal.Inc()
	PodsScheduledProgress.Set(float64(ec.scheduledCount))

	if ec.scheduledCount >= ec.remaining {
		ec.timing.LastPodScheduled = scheduledTime
		ec.computeSchedulingTime()
		ec.computeTotalDuration()
		ec.finish()
	}
}

func (ec *EventCollector) computeReactionTime() {
	if ec.timing.FirstFailedScheduling.IsZero() || ec.timing.FirstNodeCreated.IsZero() {
		return
	}
	duration := ec.timing.FirstNodeCreated.Sub(ec.timing.FirstFailedScheduling)
	ec.timing.ReactionTime = duration.String()
	ScalingDecisionLatencySeconds.Set(duration.Seconds())
}

func (ec *EventCollector) computeSchedulingTime() {
	if ec.timing.FirstNodeCreated.IsZero() || ec.timing.LastPodScheduled.IsZero() {
		return
	}
	duration := ec.timing.LastPodScheduled.Sub(ec.timing.FirstNodeCreated)
	ec.timing.SchedulingTime = duration.String()
	ScalingSchedulingLatencySeconds.Set(duration.Seconds())
}

func (ec *EventCollector) computeTotalDuration() {
	if ec.timing.FirstFailedScheduling.IsZero() || ec.timing.LastPodScheduled.IsZero() {
		return
	}
	duration := ec.timing.LastPodScheduled.Sub(ec.timing.FirstFailedScheduling)
	ec.timing.TotalDuration = duration.String()
	ScalingTotalLatencySeconds.Set(duration.Seconds())
}
