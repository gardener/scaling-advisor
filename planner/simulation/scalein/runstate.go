package scalein

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// RunState holds internal run state details of parent ScaleInSimulation.
type RunState struct {
	err                         error
	ctx                         context.Context
	view                        minkapi.View
	scaleInNodes                map[string]*corev1.Node                                   // map of node names to scale-in nodes
	unscheduledPods             map[commontypes.NamespacedName]plannerapi.PodResourceInfo // map of unscheduled Pod namespacedName to PodResourceInfo
	scheduledPodNamesByNodeName map[string]sets.Set[commontypes.NamespacedName]           // map of node names to a set of scheduled pod names
	leftoverUnscheduledPodNames sets.Set[commontypes.NamespacedName]                      // represents a set of pod names scheduled during simulation run
	status                      plannerapi.ActivityStatus
	name                        string
	traceDir                    string
	numUnchangedTrackAttempts   int
	numTrackAttempts            int
	numReceivedEvents           int
	numScheduledPods            int
	runNum                      uint32
	nodeCount                   atomic.Uint32
}

// FreshRunState returns a fresh RunState whose status is set to [plannerapi.ActivityStatusPending]
func FreshRunState() RunState {
	return RunState{
		status:                      plannerapi.ActivityStatusPending,
		scheduledPodNamesByNodeName: make(map[string]sets.Set[commontypes.NamespacedName]),
		scaleInNodes:                make(map[string]*corev1.Node),
	}
}

// Init initializes this RunState from the given params, changes the [RunState]'s [plannerapi.ActivityStatus] to
// [plannerapi.ActivityStatusRunning] and returns the child run context or an error. The view is also interrogated for
// initializing unscheduledPods. This method must be invoked before calling other
// methods of [RunState]
func (r *RunState) Init(parentCtx context.Context, name string, runNum uint32, view minkapi.View, traceDir string) (context.Context, error) {
	r.name, r.runNum, r.status, r.view, r.traceDir = name, runNum, plannerapi.ActivityStatusRunning, view, traceDir
	log := logr.FromContextOrDiscard(parentCtx).WithValues("simulationName", name, "runNum", runNum)
	r.ctx = logr.NewContext(parentCtx, log)
	unscheduledPods, err := getUnscheduledPodsMap(r.ctx, view)
	if err != nil {
		return r.ctx, fmt.Errorf("unable to get unscheduled pods from view %q: %w", view.GetName(), err)
	}
	if len(unscheduledPods) == 0 {
		return r.ctx, fmt.Errorf("no unscheduled pods in the view %q", view.GetName())
	}
	r.unscheduledPods = unscheduledPods
	r.leftoverUnscheduledPodNames = sets.New(slices.Collect(maps.Keys(unscheduledPods))...)
	return r.ctx, nil
}

func (r *RunState) GetPodsToReschedule() sets.Set[commontypes.NamespacedName] {
	return r.leftoverUnscheduledPodNames
}

func (r *RunState) GetPodNodeAssignments() ([]plannerapi.NodePodAssignment, error) {
	assignedNodeNames := slices.Collect(maps.Keys(r.scheduledPodNamesByNodeName))
	assignedNodes, err := r.view.ListNodes(r.ctx, assignedNodeNames...)
	if err != nil {
		return nil, err
	}
	otherAssignments := make([]plannerapi.NodePodAssignment, len(assignedNodes))
	for _, node := range assignedNodes {
		scheduledPodInfos := r.getScheduledPodInfosForNode(node.Name)
		if len(scheduledPodInfos) > 0 {
			nodeResources := getNodeResourceInfo(&node)
			otherAssignments = append(otherAssignments, plannerapi.NodePodAssignment{
				NodeResources: nodeResources,
				ScheduledPods: scheduledPodInfos,
			})
		}
	}
	return otherAssignments, nil
}

func (s *RunState) getScheduledPodInfosForNode(nodeName string) []plannerapi.PodResourceInfo {
	scheduledPodNames := s.scheduledPodNamesByNodeName[nodeName].UnsortedList()
	if len(scheduledPodNames) == 0 {
		return nil
	}
	scheduledPodInfos := make([]plannerapi.PodResourceInfo, 0, len(scheduledPodNames))
	for _, podName := range scheduledPodNames {
		scheduledPodInfos = append(scheduledPodInfos, s.unscheduledPods[podName])
	}
	return scheduledPodInfos
}

func getNodeResourceInfo(node *corev1.Node) plannerapi.NodeResourceInfo {
	instanceType := nodeutil.GetInstanceType(node)
	return plannerapi.NodeResourceInfo{
		Name:         node.Name,
		InstanceType: instanceType,
		Capacity:     node.Status.Capacity,
		Allocatable:  node.Status.Allocatable,
	}
}

// TODO: take care of volumes attached to this node as well
func (r *RunState) RemoveNodeAndUnbindPods(nodeName string) ([]commontypes.NamespacedName, error) {
	log := logr.FromContextOrDiscard(r.ctx)

	podListObj, err := r.view.ListObjects(r.ctx, typeinfo.PodsDescriptor.GVK, minkapi.MatchCriteria{})
	if err != nil {
		return nil, err
	}

	podList, ok := podListObj.(*corev1.PodList)
	if !ok {
		return nil, err
	}

	var unboundPods []commontypes.NamespacedName
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != nodeName {
			continue
		}

		log.V(2).Info("Unbinding pod from node", "pod", pod.Name, "node", nodeName)
		pod.Spec.NodeName = ""
		if err = r.view.UpdateObject(r.ctx, typeinfo.PodsDescriptor.GVK, &pod); err != nil {
			return nil, err
		}

		//Remove pod from scheduledPodNamesByNodeName map and to unscheduledPods map in RunState
		// It has to exist in scheduledPodNamesByNodeName, no? is this check needed?
		if scheduledPods, exists := r.scheduledPodNamesByNodeName[nodeName]; exists {
			scheduledPods.Delete(commontypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name})
		}
		r.unscheduledPods[commontypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}] = podutil.PodResourceInfoFromCoreV1Pod(&pod)

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
// attempts that exceed tbe given maxUnchangedTrackAttempts or an error.
//
// Track does the following internally:
//   - Increments numTrackAttempts and gets the last slice of events (if any) in the [minkapi.EventSink] of
//     this RunState's [minkapi.View].
//   - If the slice of events is empty, increment numUnchangedTrackAttempts.
//     If the numUnchangedTrackAttempts > maxUnchangedTrackAttempts,
//     then stabilized is considered as true and returned.
//   - If the slice of event is not empty, reset numUnchangedTrackAttempts and also invoke Reset on the
//     [minkapi.EventSink]
//   - For each "Scheduled" event in the slice of events, add the scheduled pod name to
//     scheduledPodNamesByNodeName, remove scheduled pod name from leftoverUnscheduledPodNames
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
				"numUnchangedTrackAttempts", r.numUnchangedTrackAttempts,
				"numScheduledPods", r.numScheduledPods)
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
	err = r.addScheduledPod(pod)
	return err
}

func (r *RunState) addScheduledPod(pod *corev1.Pod) error {
	log := logr.FromContextOrDiscard(r.ctx)
	podNsName := objutil.NamespacedName(pod)
	if pod.Spec.NodeName == "" {
		return fmt.Errorf("nodeName must be assigned to pod %q", podNsName)
	}
	scheduledPodNames := r.scheduledPodNamesByNodeName[pod.Spec.NodeName]
	if scheduledPodNames == nil {
		scheduledPodNames = sets.New[commontypes.NamespacedName]()
	}
	scheduledPodNames.Insert(podNsName)
	r.scheduledPodNamesByNodeName[pod.Spec.NodeName] = scheduledPodNames
	r.numScheduledPods++
	r.leftoverUnscheduledPodNames.Delete(podNsName)
	r.numUnchangedTrackAttempts = 0
	log.V(4).Info("Added scheduledPod to RunState.scheduledPodNamesByNodeName and reset numUnchangedTrackAttempts",
		"podNamespacedName", podNsName,
		"numScheduledPods", r.numScheduledPods,
		"leftoverUnscheduledPodCount", len(r.leftoverUnscheduledPodNames))
	return nil
}

func (r *RunState) NodePodAssignments(unboundPods []commontypes.NamespacedName) ([]plannerapi.NodePodAssignment, error) {
	var nodePodAssignments []plannerapi.NodePodAssignment
	for _, podNN := range unboundPods {
		podObj, err := r.view.GetObject(r.ctx, typeinfo.PodsDescriptor.GVK, cache.NewObjectName(podNN.Namespace, podNN.Name))
		if err != nil {
			return nil, err
		}
		pod, ok := podObj.(*corev1.Pod)
		if !ok {
			return nil, err
		}
		if pod.Spec.NodeName == "" {
			continue
		}
		nodePodAssignments = append(nodePodAssignments, plannerapi.NodePodAssignment{
			NodeResources: getNodeResourceInfo(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: pod.Spec.NodeName}}),
			ScheduledPods: []plannerapi.PodResourceInfo{podutil.PodResourceInfoFromCoreV1Pod(pod)},
		})
	}
	return nodePodAssignments, nil
}

func (r *RunState) GetScaleInItems() []sacorev1alpha1.ScaleInItem {
	scaleInItems := make([]sacorev1alpha1.ScaleInItem, 0, len(r.scaleInNodes))
	for _, node := range r.scaleInNodes {
		scaleInItems = append(scaleInItems, sacorev1alpha1.ScaleInItem{
			//TODO: add nodeplacement
			//NodePlacement: np,
			NodeName: node.Name,
		})
	}
	return scaleInItems
}

func getUnscheduledPodsMap(ctx context.Context, v minkapi.View) (unscheduled map[commontypes.NamespacedName]plannerapi.PodResourceInfo, err error) {
	log := logr.FromContextOrDiscard(ctx)
	pods, err := v.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return
	}
	unscheduled = make(map[commontypes.NamespacedName]plannerapi.PodResourceInfo, len(pods))
	for _, p := range pods {
		if podutil.IsUnscheduledPod(&p) {
			log.V(5).Info("found unscheduled pod", "pod", p)
			unscheduled[objutil.NamespacedName(&p)] = podutil.PodResourceInfoFromCoreV1Pod(&p)
		}
	}
	return
}
