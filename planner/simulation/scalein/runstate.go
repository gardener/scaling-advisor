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

// FreshRunState returns a zero-valued RunState whose status is [plannerapi.ActivityStatusPending].
// All other fields are left zero; the caller must invoke [RunState.Init] before any other method
// (which transitions the status to [plannerapi.ActivityStatusRunning] and populates the view,
// pod-tracking sets and snapshot of initial pod specs).
func FreshRunState() RunState {
	return RunState{
		status: plannerapi.ActivityStatusPending,
	}
}

// Init prepares this RunState for a single simulation run against the given view and must be
// called before any other RunState method. It:
//
//   - Stores the run identity (name, runNum, traceDir) and view on the receiver.
//   - Derives a child context that carries a logger annotated with simulationName and runNum;
//     the derived context is returned so callers can pass it to subsequent operations.
//   - Transitions the status from [plannerapi.ActivityStatusPending] to
//     [plannerapi.ActivityStatusRunning].
//   - Lists every pod currently in the view and snapshots a deep copy of each into
//     initialPodSpecs. This snapshot is the source of truth for handlePreemptedPodEvent when
//     re-creating a victim that the kube-scheduler has already deleted.
//   - Computes initialUnscheduledPods (the set of pods that were unscheduled at run start) and
//     seeds currentUnscheduledPods to a copy of it. IsSimulationSuccess later compares
//     currentUnscheduledPods against initialUnscheduledPods to detect newly unscheduled pods.
//   - Initializes pendingPods to an empty set; pods are inserted into it as they are unbound by
//     [RunState.RemoveNodeAndUnbindPods] or as preemption victims are observed.
//
// Returns the child context. An error is returned (and the receiver left partially populated)
// only if listing pods from the view fails.
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

// IsSimulationSuccess reports whether the simulation has reached a successful steady state.
// Two conditions must hold:
//
//   - pendingPods is empty: every pod that was unbound by [RunState.RemoveNodeAndUnbindPods]
//     or recreated by handlePreemptedPodEvent has either been rescheduled (a Scheduled event
//     was observed and removed it from pendingPods) or fell out of pendingPods via
//     handleFailedSchedulingEvent.
//   - No pod is unscheduled now that wasn't already unscheduled at run start: every entry in
//     currentUnscheduledPods must be in initialUnscheduledPods. A pod that becomes unscheduled
//     during the run (e.g., a displaced pod that the scheduler couldn't place anywhere) fails
//     this check and indicates that scaling in the candidate node would leave a workload
//     stranded.
//
// Both conditions are necessary; either one alone is insufficient.
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

// RemoveNodeAndUnbindPods simulates removing the given node from the cluster: it disassociates
// every pod scheduled on that node from the node, then deletes the node from the view. The
// kube-scheduler observing the resulting state will treat the displaced pods as unscheduled and
// attempt to reschedule them onto remaining nodes — that is the rescheduling pressure the
// scale-in simulation is designed to verify.
//
// For each pod found on the node, the behavior depends on the pod's owner:
//
//   - DaemonSet pods are deleted from the view outright. DaemonSet replicas follow the node, so
//     a DaemonSet replacement on a remaining node would already have been counted at start; we
//     do not try to reschedule them.
//   - All other pods have their bound volumes unbound (via [volutil.UnbindPodVolumes]) so the
//     kube-scheduler's VolumeBinding plugin re-evaluates volume placement, then have their
//     Spec.NodeName cleared via UpdateObject, and are inserted into pendingPods so the run
//     tracks their rescheduling outcome.
//
// Finally the node itself is deleted from the view. Returns the first error encountered while
// listing pods, unbinding volumes, updating pods, or deleting the node.
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
		if err = r.view.UpdateObject(r.ctx, typeinfo.PodsDescriptor.GVK, &pod, minkapi.ObjectOptions{}); err != nil {
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

// Track inspects the kube-scheduler events accumulated in this [RunState]'s view since the
// previous Track call and updates the simulation's pod-tracking sets accordingly. It signals
// stabilization (no progress for too long) so the caller knows the simulation has reached a
// steady state and can be terminated.
//
// Track does the following:
//   - Increments numTrackAttempts.
//   - Reads the current batch of events from the [minkapi.EventSink] of this RunState's view.
//   - If the batch is empty, increments numUnchangedTrackAttempts. When numUnchangedTrackAttempts
//     exceeds maxUnchangedTrackAttempts, the run is considered stabilized and (true, nil) is
//     returned. The caller is expected to stop iterating once stabilized is true.
//   - If the batch is non-empty, drains the EventSink (via Reset) before dispatching events; if
//     the Reset itself fails, numUnchangedTrackAttempts is cleared and the error is returned.
//   - For each event, dispatches by (Action, Reason) tuple:
//     "Binding"/"Scheduled"     -> handleScheduledPodEvent: removes the pod from pendingPods and
//     currentUnscheduledPods (the pod was successfully bound to a node).
//     "Preempting"/"Preempted"  -> handlePreemptedPodEvent: re-creates the controller-owned
//     victim pod in the view (modeling the workload controller) and tracks it in
//     pendingPods so its rescheduling outcome is observed.
//     reason == "FailedScheduling" -> handleFailedSchedulingEvent: if the pod was in pendingPods,
//     moves it to currentUnscheduledPods (kube-scheduler could not place it).
//     The dispatched handlers individually reset numUnchangedTrackAttempts when they observe
//     real progress (a state change in the tracking sets), so the stabilization counter only
//     advances during quiescent batches.
//
// Track returns (true, nil) when the run has stabilized, (false, err) on a backing-store error,
// or (false, nil) when a batch was processed and the caller should poll again.
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

// handleFailedSchedulingEvent reacts to a kube-scheduler "FailedScheduling" event. The event
// means the scheduler attempted to place the regarding pod and could not find a suitable node
// (or, in the preemption case, found candidate victims but no feasible result).
//
// If the pod is currently in pendingPods (i.e., it is one we are waiting on), it is moved to
// currentUnscheduledPods and removed from pendingPods. This advances the simulation's view of
// "displaced pods that ultimately could not be placed" — exactly the set IsSimulationSuccess
// uses to decide whether the run failed. numUnchangedTrackAttempts is reset to 0 because real
// progress was observed; pods not in pendingPods (e.g., FailedScheduling events for pods we are
// not tracking) are ignored without disturbing the stabilization counter.
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
	if _, err := r.view.CreateObject(r.ctx, typeinfo.PodsDescriptor.GVK, replacement, minkapi.ObjectOptions{}); err != nil {
		// If the pod is already present in the view (the kube-scheduler hasn't actually deleted
		// it yet, or it raced with our recreation), update it to clear NodeName instead.
		log.V(4).Info("CreateObject for preempted pod failed; falling back to clearing NodeName via UpdateObject",
			"podNamespacedName", podNsName, "err", err)
		if existingObj, gerr := r.view.GetObject(r.ctx, typeinfo.PodsDescriptor.GVK, podNsName.AsObjectName()); gerr == nil {
			if existingPod, isPod := existingObj.(*corev1.Pod); isPod {
				existingPod.Spec.NodeName = ""
				if uerr := r.view.UpdateObject(r.ctx, typeinfo.PodsDescriptor.GVK, existingPod, minkapi.ObjectOptions{}); uerr != nil {
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

// handleScheduledPodEvent reacts to a kube-scheduler "Binding/Scheduled" event. The event means
// the scheduler successfully bound the regarding pod to a node. The pod is removed from both
// pendingPods (it is no longer waiting to be rescheduled) and currentUnscheduledPods (it is no
// longer unscheduled). numUnchangedTrackAttempts is reset to 0 because real progress was
// observed. Removing from sets that don't contain the pod is a no-op, so this handler is safe
// to call for any Scheduled event regardless of whether the pod was previously tracked.
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
