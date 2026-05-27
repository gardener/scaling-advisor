package scalein

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync/atomic"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// RunState holds internal run state details of parent ScaleInSimulation.
type RunState struct {
	err                       error
	ctx                       context.Context
	view                      minkapi.View
	podsToReschedule          sets.Set[commontypes.NamespacedName] // represents a set of pod names which would need to be rescheduled because of scaling-in the node
	status                    plannerapi.ActivityStatus
	name                      string
	traceDir                  string
	numUnchangedTrackAttempts int
	numTrackAttempts          int
	numReceivedEvents         int
	runNum                    uint32
	nodeCount                 atomic.Uint32
}

// FreshRunState returns a fresh RunState whose status is set to [plannerapi.ActivityStatusPending]
func FreshRunState() RunState {
	return RunState{
		status: plannerapi.ActivityStatusPending,
	}
}

// Init initializes this RunState from the given params, changes the [RunState]'s [plannerapi.ActivityStatus] to
// [plannerapi.ActivityStatusRunning] and returns the child run context or an error.
// This method must be invoked before calling other methods of [RunState]
func (r *RunState) Init(parentCtx context.Context, name string, runNum uint32, view minkapi.View, traceDir string, nodeName string) (context.Context, error) {
	r.name, r.runNum, r.status, r.view, r.traceDir = name, runNum, plannerapi.ActivityStatusRunning, view, traceDir
	log := logr.FromContextOrDiscard(parentCtx).WithValues("simulationName", name, "runNum", runNum)
	r.ctx = logr.NewContext(parentCtx, log)
	unscheduledPods, err := getUnscheduledPodsMap(r.ctx, view, nodeName)
	if err != nil {
		return r.ctx, fmt.Errorf("unable to get unscheduled pods from view %q: %w", view.GetName(), err)
	}
	r.podsToReschedule = sets.New(slices.Collect(maps.Keys(unscheduledPods))...)
	return r.ctx, nil
}

func (r *RunState) GetPodsToReschedule() sets.Set[commontypes.NamespacedName] {
	return r.podsToReschedule
}

func (r *RunState) RemoveNodeAndUnbindPods(nodeName string) ([]commontypes.NamespacedName, error) {
	log := logr.FromContextOrDiscard(r.ctx)

	pods, err := viewutil.ListPodsOfNode(r.ctx, r.view, nodeName)
	if err != nil {
		return nil, err
	}

	var unboundPods []commontypes.NamespacedName
	for _, pod := range pods {
		if isDaemonSetPod(pod) {
			if err = r.view.DeleteObject(r.ctx, typeinfo.PodsDescriptor.GVK, cache.NewObjectName(pod.Namespace, pod.Name)); err != nil {
				return nil, err
			}
			continue
		}

		if err = volutil.UnbindPodVolumes(r.ctx, r.view, &pod); err != nil {
			return nil, err
		}

		log.V(2).Info("Unbinding pod from node", "pod", pod.Name, "node", nodeName)
		pod.Spec.NodeName = ""
		if err = r.view.UpdateObject(r.ctx, typeinfo.PodsDescriptor.GVK, &pod); err != nil {
			return nil, err
		}

		unboundPods = append(unboundPods, commontypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name})
	}

	// Delete node from view
	err = r.view.DeleteObject(r.ctx, typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nodeName))
	if err != nil {
		return nil, err
	}
	return unboundPods, nil
}

// Track is used to track the RunState of the simulation by recording the pod-node binding(s) if any made in this
// [RunState]'s view by the `kube-scheduler`. It returns true if the RunState has not changed over many Track
// attempts that exceed the given maxUnchangedTrackAttempts or an error.
//
// Track does the following internally:
//   - Increments numTrackAttempts and gets the last slice of events (if any) in the [minkapi.EventSink] of
//     this RunState's [minkapi.View].
//   - If the slice of events is empty, increment numUnchangedTrackAttempts.
//     If the numUnchangedTrackAttempts > maxUnchangedTrackAttempts,
//     then stabilized is considered as true and returned.
//   - If the slice of event is not empty, reset numUnchangedTrackAttempts and also invoke Reset on the
//     [minkapi.EventSink]
//   - For each "Scheduled" event in the slice of events, remove scheduled pod name from podsToReschedule
func (r *RunState) Track(maxUnchangedTrackAttempts int) (stabilized bool, err error) {
	log := logr.FromContextOrDiscard(r.ctx)
	r.numTrackAttempts++
	evList := r.view.GetEventSink().List()
	log.V(4).Info("Track Invoked", "numEvents", len(evList),
		"numTrackAttempts", r.numTrackAttempts,
		"numUnchangedTrackAttempts", r.numUnchangedTrackAttempts,
		"maxUnchangedTrackAttempts", maxUnchangedTrackAttempts)
	if len(evList) == 0 {
		r.numUnchangedTrackAttempts++
		if r.numUnchangedTrackAttempts > maxUnchangedTrackAttempts {
			log.V(3).Info("simulation RunState stabilized - no new kube-scheduler events observed",
				"numReceivedEvents", r.numReceivedEvents,
				"maxUnchangedTrackAttempts", maxUnchangedTrackAttempts,
				"numUnchangedTrackAttempts", r.numUnchangedTrackAttempts)
			stabilized = true
		}
		return
	} else if err = r.view.GetEventSink().Reset(); err != nil {
		r.numUnchangedTrackAttempts = 0
		return
	}

	for idx, ev := range evList {
		var eventTime = ev.EventTime
		if ev.Series != nil {
			eventTime = ev.Series.LastObservedTime
		}
		log.V(5).Info("Checking event", "index", idx, "id", ev.UID, "eventTime", eventTime,
			"ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance,
			"Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding, "Note", ev.Note)
		r.numReceivedEvents++
		if ev.Action != "Binding" && ev.Reason != "Scheduled" {
			if ev.Reason == "FailedScheduling" {
				log.V(4).Info("FailedScheduling event", "index", idx, "id", ev.UID,
					"ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance,
					"Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding, "Note", ev.Note)
			}
			continue
		}
		if err = r.handleScheduledPodEvent(ev); err != nil {
			return
		}
	}

	return
}

func (r *RunState) handleScheduledPodEvent(ev eventsv1.Event) error {
	log := logr.FromContextOrDiscard(r.ctx)
	podNsName := objutil.NamespacedNameFromEventRegarding(ev)
	log.V(4).Info("PodScheduled event.", "podNamespacedName", podNsName, "eventNote", ev.Note)
	obj, err := r.view.GetObject(r.ctx, typeinfo.PodsDescriptor.GVK, podNsName.AsObjectName())
	if err != nil {
		return err
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("object %T and name %q is not a Pod", pod, podNsName)
	}
	if pod.Spec.NodeName == "" {
		return fmt.Errorf("scheduledPod %q has no assigned node name even with binding event note %q", podNsName, ev.Note)
	}
	err = r.removeFromPodsToReschedule(pod)
	return err
}

func (r *RunState) removeFromPodsToReschedule(pod *corev1.Pod) error {
	log := logr.FromContextOrDiscard(r.ctx)
	podNsName := objutil.NamespacedName(pod)
	if pod.Spec.NodeName == "" {
		return fmt.Errorf("nodeName must be assigned to pod %q", podNsName)
	}
	r.podsToReschedule.Delete(podNsName)
	r.numUnchangedTrackAttempts = 0
	log.V(4).Info("Removed pod from RunState.podsToReschedule and reset numUnchangedTrackAttempts",
		"podNamespacedName", podNsName,
		"podsToRescheduleCount", len(r.podsToReschedule))
	return nil
}

func getUnscheduledPodsMap(ctx context.Context, v minkapi.View, nodeName string) (unscheduled map[commontypes.NamespacedName]plannerapi.PodResourceInfo, err error) {
	log := logr.FromContextOrDiscard(ctx)
	pods, err := viewutil.ListPodsOfNode(ctx, v, nodeName)
	if err != nil {
		return
	}
	unscheduled = make(map[commontypes.NamespacedName]plannerapi.PodResourceInfo, len(pods))
	for _, p := range pods {
		if !isDaemonSetPod(p) {
			log.V(5).Info("found non-daemonset pod attached to scale-in node", "pod", p)
			unscheduled[objutil.NamespacedName(&p)] = podutil.PodResourceInfoFromCoreV1Pod(&p)
		}
	}
	return
}

func isDaemonSetPod(pod corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}
