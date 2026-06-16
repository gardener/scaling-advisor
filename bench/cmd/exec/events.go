// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"log"
	"math"
	"slices"
	"time"

	benchutil "github.com/gardener/scaling-advisor/bench/cmd/util"

	pricingapi "github.com/gardener/scaling-advisor/api/pricing"
	"github.com/gardener/scaling-advisor/common/objutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

// NewEventCollector creates an EventCollector that watches for scaling events.
func NewEventCollector(res *resources.Resources, unscheduledCount int, eventConfig ScalerEventConfig) *EventCollector {
	// log.Printf("DEBUG: Unsched counter initial: %d\n", unscheduledCount)
	return &EventCollector{
		res:                res,
		unscheduledCounter: unscheduledCount,
		eventConfig:        eventConfig,
		done:               make(chan struct{}),
	}
}

// Start begins three watches: Events, Nodes (Add/Update/Delete) and
// Pods (Scheduled/Deleted).
func (ec *EventCollector) Start(ctx context.Context) error {
	// FIXME: this breaks scale-in testing
	// if ec.unscheduledCounter <= 0 {
	// 	ec.finish()
	// 	return nil
	// }

	existingNodes := &corev1.NodeList{}
	if err := ec.res.List(ctx, existingNodes); err != nil {
		return err
	}
	existingNodeNames := make(map[string]bool, len(existingNodes.Items))
	for _, n := range existingNodes.Items {
		existingNodeNames[n.Name] = true
	}

	ec.unschedulablePods = sets.New[string]()
	ec.podSchedulingDurations = make(map[string][]podSchedulingDuration)

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

// watchEvents looks for:
//  1. first 'FailedScheduling' event raised by the scheduler in order to start tracking
//     all reaction and scheduling timing.
//  2. 'Preempted' events to know which pods will be recreated.
//  3. scaler specific events ('MarksPodsUnschedulable') to find pods which are marked as
//     unschedulable hence won't be triggering further scale-ups.
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
		// Capture the first 'FailedScheduling' event from the scheduler
		// used for time tracking purposes
		if event.Reason == "FailedScheduling" &&
			(source != "cluster-autoscaler" && source != "karpenter") {
			if ec.timing.FirstFailedScheduling.IsZero() {
				ec.timing.FirstFailedScheduling = event.CreationTimestamp.UTC()
				ec.events = append(ec.events, ScalingEvent{
					Timestamp: event.CreationTimestamp.UTC(),
					Type:      event.Reason,
					Source:    source,
					Name:      event.InvolvedObject.Name,
					Namespace: event.InvolvedObject.Namespace,
					Details:   event.Message,
				})
			}
		} else if event.Reason == "Preempted" {
			ec.events = append(ec.events, ScalingEvent{
				Timestamp: event.CreationTimestamp.UTC(),
				Type:      event.Reason,
				Source:    source,
				Name:      event.InvolvedObject.Name,
				Namespace: event.InvolvedObject.Namespace,
				Details:   event.Message,
			})
			log.Printf("%s | %s : %s", event.Reason, event.InvolvedObject.Name, event.Message)
		} else if source == cfg.Source && slices.Contains(cfg.WatchedEvents, event.Reason) {
			ec.processScalerEvent(ctx, cfg, source, event)
		} else if event.Reason == "Scheduled" {
			log.Printf("%s | %s : %s", event.Reason, event.InvolvedObject.Name, event.Message)
		}
	})
	if err := ew.Start(ctx); err != nil {
		return err
	}
	ec.watchers = append(ec.watchers, ew)
	return nil
}

func (ec *EventCollector) processScalerEvent(ctx context.Context, eCfg ScalerEventConfig, source string, event *corev1.Event) {
	// Karpenter produces a very hefty message for its 'FailedScheduling' events
	// detailing out all the constraints that failed, this can bloat up the events
	// file. If this information is needed, logs can be checked.
	message := event.Message
	if event.Reason == "FailedScheduling" && source == benchutil.ScalerKarpenter {
		message = "Failed to scheduled pod"
	}

	ec.events = append(ec.events, ScalingEvent{
		Timestamp: event.CreationTimestamp.UTC(),
		Type:      event.Reason,
		Source:    source,
		Name:      event.InvolvedObject.Name,
		Namespace: event.InvolvedObject.Namespace,
		Details:   message,
	})

	log.Printf("%s | %s : %s", event.Reason, event.InvolvedObject.Name, message)

	if slices.Contains(eCfg.MarksPodUnschedulable, event.Reason) {
		key := event.InvolvedObject.Namespace + "/" + event.InvolvedObject.Name
		var pod corev1.Pod
		err := ec.res.Get(ctx, event.InvolvedObject.Name, event.InvolvedObject.Namespace, &pod)
		if err != nil {
			log.Printf("ERR: could not fetch pod %q: %s", key, err.Error())
			return
		}
		// If its a daemonset pod, then its not tracked
		if !isOwner(pod.GetOwnerReferences(), benchutil.OwnerDaemonSet) {
			ec.podUnschedulable(key)
		}
	}
}

// watchNodes collects:
//  1. creation events for non existing nodes.
//  2. delete events to know which nodes might've been scaled-in.
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

		ec.events = append(ec.events, ScalingEvent{
			Timestamp: node.CreationTimestamp.UTC(),
			Type:      "NodeCreated",
			Source:    "node-watch",
			Name:      node.Name,
			Details:   node.Labels[corev1.LabelInstanceTypeStable],
			Region:    node.Labels[corev1.LabelTopologyRegion],
		})
		ec.timing.LastScaleOutTime = node.CreationTimestamp.UTC()

		if ec.timing.FirstNodeCreated.IsZero() {
			ec.timing.FirstNodeCreated = node.CreationTimestamp.UTC()
			ec.computeReactionTime()
		}
		NodesCreatedTotal.Inc()
	})
	nw.WithDeleteFunc(func(obj any) {
		node, ok := obj.(*corev1.Node)
		if !ok {
			return
		}
		ec.mu.Lock()
		defer ec.mu.Unlock()

		ec.events = append(ec.events, ScalingEvent{
			Timestamp: time.Now().UTC(),
			Type:      "NodeDeleted",
			Source:    "node-watch",
			Name:      node.Name,
			Details:   node.Labels[corev1.LabelInstanceTypeStable],
			Region:    node.Labels[corev1.LabelTopologyRegion],
		})
		if node.GetDeletionTimestamp() != nil {
			ec.timing.LastScaleInTime = node.DeletionTimestamp.UTC()
		} else {
			ec.timing.LastScaleInTime = time.Now().UTC()
		}
		NodesDeletedTotal.Inc()
	})

	if err := nw.Start(ctx); err != nil {
		return err
	}
	ec.watchers = append(ec.watchers, nw)
	return nil
}

// watchPods collects:
//  1. update events to find pods which got scheduled.
//  2. delete events to re-create non daemonset/job pods.
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

		// Check if this pod (having the same 'name' and 'UID') has already been tracked
		key := objutil.NamespacedName(pod).String()
		if latencies, exists := ec.podSchedulingDurations[key]; exists {
			if slices.ContainsFunc(latencies, func(latency podSchedulingDuration) bool {
				return latency.UID == string(pod.UID)
			}) {
				return
			}
		}
		ec.podScheduled(pod)
	})
	pw.WithDeleteFunc(func(obj any) {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return
		}
		// If a pod is deleted and its a daemonset or a job pod, then its not re-created
		if isOwner(pod.GetOwnerReferences(), benchutil.OwnerDaemonSet) || isOwner(pod.GetOwnerReferences(), benchutil.OwnerJob) {
			return
		}

		ec.mu.Lock()
		defer ec.mu.Unlock()
		// If nodename is non-empty, i.e. pod was already scheduled, only then increment
		// the total unscheduled pods counter
		if pod.Spec.NodeName != "" {
			ec.unscheduledCounter++
			// log.Printf("DEBUG: Unsched counter: %d\n", ec.unscheduledCounter)
			pod.Spec.NodeName = ""
		}
		pod.ResourceVersion = ""
		pod.UID = ""
		pod.DeletionTimestamp = nil
		pod.Status = corev1.PodStatus{}
		if err := ec.res.Create(ctx, pod); err != nil {
			log.Printf("ERR: could not recreate pod %q: %s", pod.Name, err.Error())
		} else {
			log.Printf("Recreated deleted pod: %q", pod.Name)
		}
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
func (ec *EventCollector) Results(pricingData pricingapi.InstancePricingAccess) ([]ScalingEvent, TimingBreakdown, Summary) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	countByType := make(map[string]int)
	instanceTypes := make(map[string]InstanceDetails)
	var (
		failures         []ScalingFailure
		totalHourlyPrice float64
	)

	var timeline []ScalingEvent
	for _, e := range ec.events {
		countByType[e.Type]++
		if e.Type == "NodeCreated" && e.Details != "" {
			instanceDetails := instanceTypes[e.Details]
			if instanceDetails.Count == 0 {
				instancePricing, _ := pricingData.GetInfo(e.Region, e.Details)
				instanceDetails.Price = instancePricing.HourlyPrice
				instanceDetails.Region = e.Region
			}
			instanceDetails.Count++
			totalHourlyPrice += instanceDetails.Price
			instanceTypes[e.Details] = instanceDetails
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

	schedulingDurations := make(map[string][]string, len(ec.podSchedulingDurations))
	if len(ec.podSchedulingDurations) > 0 {
		for name, duration := range ec.podSchedulingDurations {
			for _, d := range duration {
				schedulingDurations[name] = append(schedulingDurations[name], d.TimeToSchedule.String())
			}
		}
	}

	summary := Summary{
		Events: EventsSummary{
			TotalCount:  len(ec.events),
			CountByType: countByType,
		},
		Nodes: NodesSummary{
			TotalCreated:     countByType["NodeCreated"],
			InstanceTypes:    instanceTypes,
			TotalHourlyPrice: totalHourlyPrice,
		},
		Pods: PodsSummary{
			UnschedulablePods:   ec.unschedulablePods.Len(),
			SchedulingLatency:   ec.computeSchedulingLatency(),
			SchedulingDurations: schedulingDurations,
			Failures:            failures,
		},
	}
	log.Printf("%+v\n", summary.Nodes)

	return timeline, ec.timing, summary
}

// exitCriteriaMet checks if the sum of number of unschedulable pods (that couldn't trigger scale up)
// and scheduled pods meets or exceeds the total number of unscheduled pods we are waiting for
func (ec *EventCollector) exitCriteriaMet() bool {
	return ec.unschedulablePods.Len()+ec.scheduledCount >= ec.unscheduledCounter
}

func (ec *EventCollector) podUnschedulable(podName string) {
	if ec.unschedulablePods.Has(podName) {
		return
	}
	ec.unschedulablePods.Insert(podName)
	log.Printf("Pod %q marked unschedulable\n", podName)
	// log.Printf("DEBUG: Unschedulable: %d\n", len(ec.unschedulablePods))

	if ec.exitCriteriaMet() {
		ec.timing.LastPodResolved = time.Now().UTC()
		ec.computeScalingTimes()
		ec.computeSchedulingTime()
		ec.finish()
	}
}

func (ec *EventCollector) podScheduled(pod *corev1.Pod) {
	key := objutil.NamespacedName(pod).String()

	scheduledTime := time.Now().UTC()
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
			scheduledTime = cond.LastTransitionTime.UTC()
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

	ec.podSchedulingDurations[key] = append(ec.podSchedulingDurations[key], podSchedulingDuration{
		UID:            string(pod.UID),
		TimeToSchedule: scheduledTime.Sub(pod.CreationTimestamp.UTC()),
	})

	// Only increment counter for non daemonset pods
	if !isOwner(pod.GetOwnerReferences(), benchutil.OwnerDaemonSet) {
		ec.scheduledCount++
	}
	// log.Printf("DEBUG: Sched counter: %d\n", ec.scheduledCount)
	PodsScheduledTotal.Inc()

	if ec.exitCriteriaMet() {
		ec.timing.LastPodResolved = scheduledTime
		ec.computeScalingTimes()
		ec.computeSchedulingTime()
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
	if len(ec.podSchedulingDurations) == 0 {
		return SchedulingLatency{}
	}

	durations := make([]time.Duration, 0, len(ec.podSchedulingDurations))
	for _, latencies := range ec.podSchedulingDurations {
		for _, l := range latencies {
			if l.TimeToSchedule > 0 {
				durations = append(durations, l.TimeToSchedule)
			}
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

func (ec *EventCollector) computeScalingTimes() {
	if !ec.timing.FirstFailedScheduling.IsZero() && !ec.timing.LastScaleInTime.IsZero() {
		duration := ec.timing.LastScaleInTime.Sub(ec.timing.FirstFailedScheduling)
		ec.timing.ScaleInTime = duration.String()
	}
	if !ec.timing.FirstFailedScheduling.IsZero() && !ec.timing.LastScaleOutTime.IsZero() {
		duration := ec.timing.LastScaleOutTime.Sub(ec.timing.FirstFailedScheduling)
		ec.timing.ScaleOutTime = duration.String()
	}
}

func (ec *EventCollector) computeSchedulingTime() {
	if ec.timing.FirstNodeCreated.IsZero() || ec.timing.LastPodResolved.IsZero() {
		return
	}
	duration := ec.timing.LastPodResolved.Sub(ec.timing.FirstNodeCreated)
	ec.timing.SchedulingTime = duration.String()
}

func isOwner(owners []metav1.OwnerReference, kind string) bool {
	if owners == nil {
		return false
	}
	if slices.ContainsFunc(owners, func(owner metav1.OwnerReference) bool {
		return owner.Kind == kind
	}) {
		return true
	}
	return false
}
