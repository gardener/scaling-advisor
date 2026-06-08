package scalein

import (
	"context"
	"fmt"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/common/viewutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// RunState holds internal run state details of parent ScaleInSimulation.
type RunState struct {
	err                    error
	ctx                    context.Context
	view                   minkapi.View
	initialUnscheduledPods sets.Set[commontypes.NamespacedName]
	pendingPods            sets.Set[commontypes.NamespacedName]
	currentUnscheduledPods sets.Set[commontypes.NamespacedName]
	// initialPodSpecs caches a deep copy of every pod that exists in the view at simulation
	// start, keyed by namespaced name. It lets handlePreemptedPodEvent re-create the spec of a
	// victim pod that the kube-scheduler has already deleted, modeling a controller (Deployment,
	// StatefulSet, etc.) that would re-create the pod in a real cluster.
	initialPodSpecs           map[commontypes.NamespacedName]*corev1.Pod
	status                    plannerapi.ActivityStatus
	name                      string
	traceDir                  string
	numUnchangedTrackAttempts int
	numTrackAttempts          int
	numReceivedEvents         int
	runNum                    uint32
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
func (r *RunState) Init(parentCtx context.Context, name string, runNum uint32, view minkapi.View, traceDir string) (context.Context, error) {
	r.name, r.runNum, r.status, r.view, r.traceDir = name, runNum, plannerapi.ActivityStatusRunning, view, traceDir
	log := logr.FromContextOrDiscard(parentCtx).WithValues("simulationName", name, "runNum", runNum)
	r.ctx = logr.NewContext(parentCtx, log)
	allPods, err := view.ListPods(r.ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return r.ctx, fmt.Errorf("unable to list pods from view %q: %w", view.GetName(), err)
	}
	r.initialPodSpecs = make(map[commontypes.NamespacedName]*corev1.Pod, len(allPods))
	r.initialUnscheduledPods = make(sets.Set[commontypes.NamespacedName])
	for i := range allPods {
		p := allPods[i]
		nsName := objutil.NamespacedName(&p)
		r.initialPodSpecs[nsName] = p.DeepCopy()
		if podutil.IsUnscheduledPod(&p) {
			log.V(5).Info("found unscheduled pod", "pod", p)
			r.initialUnscheduledPods.Insert(nsName)
		}
	}
	r.currentUnscheduledPods = r.initialUnscheduledPods.Union(nil)
	r.pendingPods = sets.New[commontypes.NamespacedName]()
	return r.ctx, nil
}

// IsSimulationSuccess reports whether all displaced pods were successfully rescheduled.
func (r *RunState) IsSimulationSuccess() bool {
	if r.pendingPods.Len() > 0 {
		return false
	}
	for unscheduledPod := range r.currentUnscheduledPods {
		if !r.initialUnscheduledPods.Has(unscheduledPod) {
			return false
		}
	}
	return true
}

// RemoveNodeAndUnbindPods removes the node from the view and unbinds all pods scheduled on it.
func (r *RunState) RemoveNodeAndUnbindPods(nodeName string) error {
	log := logr.FromContextOrDiscard(r.ctx)

	pods, err := viewutil.ListPodsOfNode(r.ctx, r.view, nodeName)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if podutil.IsDaemonSetPod(pod) {
			if err = r.view.DeleteObject(r.ctx, typeinfo.PodsDescriptor.GVK, cache.NewObjectName(pod.Namespace, pod.Name)); err != nil {
				return err
			}
			continue
		}

		if err = volutil.UnbindPodVolumes(r.ctx, r.view, &pod); err != nil {
			return err
		}

		log.V(2).Info("Unbinding pod from node", "pod", pod.Name, "node", nodeName)
		pod.Spec.NodeName = ""
		if err = r.view.UpdateObject(r.ctx, typeinfo.PodsDescriptor.GVK, &pod); err != nil {
			return err
		}

		r.pendingPods.Insert(commontypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name})
	}

	// Delete node from view
	err = r.view.DeleteObject(r.ctx, typeinfo.NodesDescriptor.GVK, cache.NewObjectName("", nodeName))
	if err != nil {
		return err
	}
	return nil
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
		switch {
		case ev.Action == "Binding" && ev.Reason == "Scheduled":
			r.handleScheduledPodEvent(ev)
		case ev.Action == "Preempting" && ev.Reason == "Preempted":
			r.handlePreemptedPodEvent(ev)
		case ev.Reason == "FailedScheduling":
			r.handleFailedSchedulingEvent(ev)
		}
	}

	return
}

func (r *RunState) handleFailedSchedulingEvent(ev eventsv1.Event) {
	log := logr.FromContextOrDiscard(r.ctx)
	podNsName := objutil.NamespacedNameFromEventRegarding(ev)
	log.V(4).Info("FailedScheduling pod event", "podNamespacedName", podNsName, "eventNote", ev.Note)
	if r.pendingPods.Has(podNsName) {
		r.numUnchangedTrackAttempts = 0
		r.pendingPods.Delete(podNsName)
		r.currentUnscheduledPods.Insert(podNsName)
		log.V(4).Info("Removed pod from RunState.pendingPods and added to currentUnscheduledPods on FailedScheduling",
			"podNamespacedName", podNsName, "pendingPodsCount", r.pendingPods.Len())
	}
}

// handlePreemptedPodEvent reacts to a kube-scheduler preemption event by simulating the
// recreation of the victim pod that the scheduler has just deleted. In a real cluster, when
// kube-scheduler preempts a pod, it issues a DELETE on the victim. The owning controller
// (Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, ...) then re-creates a replacement pod
// which re-enters the scheduling queue. The simulation has no live controllers, so this method
// stands in for them: for any controller-owned victim, it re-creates the pod in the view with a
// cleared NodeName and a fresh ObjectMeta so the kube-scheduler will attempt to schedule it
// again. Bare pods (no controller) are left deleted, matching real-cluster behavior. The pod is
// added to pendingPods so the simulation tracks its rescheduling outcome — if the kube-scheduler
// later successfully binds it, handleScheduledPodEvent removes it; if scheduling fails,
// handleFailedSchedulingEvent moves it to currentUnscheduledPods and IsSimulationSuccess fails.
func (r *RunState) handlePreemptedPodEvent(ev eventsv1.Event) {
	log := logr.FromContextOrDiscard(r.ctx)
	podNsName := objutil.NamespacedNameFromEventRegarding(ev)
	log.V(4).Info("Preempted pod event", "podNamespacedName", podNsName, "eventNote", ev.Note)
	r.numUnchangedTrackAttempts = 0

	originalSpec, ok := r.initialPodSpecs[podNsName]
	if !ok {
		// No record of this pod at simulation start — nothing to recreate. Track it in
		// pendingPods so the run can't claim success without observing a Scheduled event.
		log.V(4).Info("Preempted pod has no recorded initial spec; tracking as pending without recreation",
			"podNamespacedName", podNsName)
		r.pendingPods.Insert(podNsName)
		return
	}
	if !podutil.HasControllerOwner(originalSpec) {
		// Bare pods are not recreated by any controller in real clusters; they are simply gone
		// after preemption. Don't add to pendingPods — there is no future scheduling event to
		// observe, and pretending otherwise would deadlock IsSimulationSuccess.
		log.V(4).Info("Preempted pod has no controller owner; treating as gone (no recreation)",
			"podNamespacedName", podNsName)
		return
	}

	// Recreate the pod so the kube-scheduler observes it and attempts to schedule it elsewhere.
	// Reset NodeName, UID and ResourceVersion so the view treats this as a fresh object.
	replacement := originalSpec.DeepCopy()
	replacement.Spec.NodeName = ""
	replacement.UID = ""
	replacement.ResourceVersion = ""
	replacement.Status = corev1.PodStatus{}
	if _, err := r.view.CreateObject(r.ctx, typeinfo.PodsDescriptor.GVK, replacement); err != nil {
		// If the pod is already present in the view (the kube-scheduler hasn't actually deleted
		// it yet, or it raced with our recreation), update it to clear NodeName instead.
		log.V(4).Info("CreateObject for preempted pod failed; falling back to clearing NodeName via UpdateObject",
			"podNamespacedName", podNsName, "err", err)
		if existingObj, gerr := r.view.GetObject(r.ctx, typeinfo.PodsDescriptor.GVK, podNsName.AsObjectName()); gerr == nil {
			if existingPod, isPod := existingObj.(*corev1.Pod); isPod {
				existingPod.Spec.NodeName = ""
				if uerr := r.view.UpdateObject(r.ctx, typeinfo.PodsDescriptor.GVK, existingPod); uerr != nil {
					log.V(2).Info("UpdateObject fallback for preempted pod also failed",
						"podNamespacedName", podNsName, "err", uerr)
				}
			}
		}
	}
	r.pendingPods.Insert(podNsName)
	log.V(4).Info("Recreated preempted pod and tracked it in pendingPods",
		"podNamespacedName", podNsName,
		"pendingPodsCount", len(r.pendingPods))
}

func (r *RunState) handleScheduledPodEvent(ev eventsv1.Event) {
	log := logr.FromContextOrDiscard(r.ctx)
	podNsName := objutil.NamespacedNameFromEventRegarding(ev)
	log.V(4).Info("PodScheduled event.", "podNamespacedName", podNsName, "eventNote", ev.Note)
	r.currentUnscheduledPods.Delete(podNsName)
	r.pendingPods.Delete(podNsName)
	r.numUnchangedTrackAttempts = 0
	log.V(4).Info("Removed pod from RunState.podsToReschedule, RunState.pendingPods and reset numUnchangedTrackAttempts",
		"podNamespacedName", podNsName,
		"podsToRescheduleCount", len(r.currentUnscheduledPods))
}
