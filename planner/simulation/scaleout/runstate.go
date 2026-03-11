// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scaleout

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync/atomic"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/minkapi/typeinfo"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// RunState holds internal run state details of parent ScaleOutSimulation.
type RunState struct {
	err                         error
	ctx                         context.Context
	view                        minkapi.View
	scaleOutNodes               map[string]*corev1.Node                                   // map of node names to scale-out nodes
	scaleOutPlacements          map[sacorev1alpha1.NodePlacement]int32                    // map of NodePlacement's to counts
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

// MakeRunState returns a fresh RunState whose status is set to [plannerapi.ActivityStatusPending]
func MakeRunState() RunState {
	return RunState{
		status:                      plannerapi.ActivityStatusPending,
		scheduledPodNamesByNodeName: make(map[string]sets.Set[commontypes.NamespacedName]),
		scaleOutNodes:               make(map[string]*corev1.Node),
		scaleOutPlacements:          make(map[sacorev1alpha1.NodePlacement]int32),
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

// CreateSimulationNodes creates one or more scale-out simulation node(s) and associated CSI node(s)
// according to the given [plannerapi.ScaleOutNodeTemplate](s).
func (r *RunState) CreateSimulationNodes(storageMetaAccess plannerapi.StorageMetaAccess, nodeTemplates []plannerapi.ScaleOutNodeTemplate) error {
	log := logr.FromContextOrDiscard(r.ctx)
	numCreated := 0
	for _, nodeTemplate := range nodeTemplates {
		scaleOutSimNode, err := r.createNode(nodeTemplate)
		if err != nil {
			return err
		}
		numCreated++
		if err = r.createCSINode(storageMetaAccess, scaleOutSimNode); err != nil {
			return err
		}
	}
	log.V(2).Info("CreateSimulationNodes created ScaleOutSimNode(s)", "numCreated", numCreated)
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

// GetScaleOutItems returns the slice of [sacorev1alpha1.ScaleOutItem] where each item
// encapsulates the [sacorev1alpha1.NodePlacement] and associated delta.
func (r *RunState) GetScaleOutItems() []sacorev1alpha1.ScaleOutItem {
	scaleOutItems := make([]sacorev1alpha1.ScaleOutItem, 0, len(r.scaleOutPlacements))
	for np, delta := range r.scaleOutPlacements {
		scaleOutItems = append(scaleOutItems, sacorev1alpha1.ScaleOutItem{
			NodePlacement: np,
			Delta:         delta,
		})
	}
	return scaleOutItems
}

func (r *RunState) createNode(nodeTemplate plannerapi.ScaleOutNodeTemplate) (*corev1.Node, error) {
	node := r.buildScaleOutSimNode(nodeTemplate)
	simNodeRuntimeObj, err := r.view.CreateObject(r.ctx, typeinfo.NodesDescriptor.GVK, node)
	if err != nil {
		return nil, err
	}
	node = simNodeRuntimeObj.(*corev1.Node)
	log := logr.FromContextOrDiscard(r.ctx)
	log.V(2).Info("created ScaleOutSimNode",
		"scaleOutSimNodeName", node.Name,
		"UID", node.UID,
		"instanceType", node.Labels[corev1.LabelInstanceTypeStable],
		"region", node.Labels[corev1.LabelTopologyRegion],
		"availabilityZone", node.Labels[corev1.LabelTopologyZone],
		"capacity", node.Status.Capacity,
		"allocatable", node.Status.Allocatable,
		"numUnscheduledPods", len(r.unscheduledPods))
	if err = logutil.DumpObjectIfNeeded(r.ctx, node); err != nil {
		return nil, err
	}
	r.scaleOutNodes[node.Name] = node
	r.scaleOutPlacements[nodeTemplate.NodePlacement]++
	return node, nil
}

func (r *RunState) createCSINode(storageMetaAccess plannerapi.StorageMetaAccess, scaleOutSimNode *corev1.Node) error {
	csiNodeSpec, err := storageMetaAccess.GetFallbackCSINodeSpec(scaleOutSimNode.Labels[corev1.LabelInstanceTypeStable])
	if err != nil {
		return err
	}
	for i := range csiNodeSpec.Drivers {
		csiNodeSpec.Drivers[i].NodeID = scaleOutSimNode.Name
	}
	csiNode := nodeutil.NewCSINode(scaleOutSimNode.Name, scaleOutSimNode.UID, csiNodeSpec)
	runtimeObj, err := r.view.CreateObject(r.ctx, typeinfo.CSINodeDescriptor.GVK, csiNode)
	if err != nil {
		return err
	}
	csiNode = runtimeObj.(*storagev1.CSINode)
	log := logr.FromContextOrDiscard(r.ctx)
	log.V(4).Info("created CSINode for ScaleOutSimNode", "name", csiNode.GetName(), "ownerReferences", csiNode.GetOwnerReferences())
	if err = logutil.DumpObjectIfNeeded(r.ctx, csiNode); err != nil {
		return err
	}
	return err
}

func (r *RunState) buildScaleOutSimNode(nodeTemplate plannerapi.ScaleOutNodeTemplate) *corev1.Node {
	scaleOutNodeName := fmt.Sprintf("ScaleOutSimNode-%d_%d_%s_%s_%s",
		r.nodeCount.Add(1), r.runNum, nodeTemplate.PoolName, nodeTemplate.TemplateName, nodeTemplate.AvailabilityZone)
	nodeTaints := slices.Clone(nodeTemplate.Taints)
	nodeLabels := make(map[string]string)
	nodeLabels[commonconstants.LabelSimulationRunNum] = fmt.Sprintf("%d", r.runNum)
	nodeutil.AddNodeLabels(nodeLabels, nodeTemplate.Architecture, scaleOutNodeName, nodeTemplate.NodePlacement)
	nodeLabels["topology.ebs.csi.aws.com/zone"] = nodeTemplate.AvailabilityZone // TODO: need this for edge cases
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   scaleOutNodeName,
			Labels: nodeLabels,
		},
		Spec: corev1.NodeSpec{
			ProviderID: scaleOutNodeName,
			Taints:     nodeTaints,
		},
		Status: corev1.NodeStatus{
			Capacity:    nodeTemplate.Capacity,
			Allocatable: nodeutil.BuildAllocatable(nodeTemplate.Capacity, nodeTemplate.SystemReserved, nodeTemplate.KubeReserved),
			Conditions:  nodeutil.BuildReadyConditions(time.Now()),
		},
	}
}

func (r *RunState) getScheduledPodInfosForNode(nodeName string) []plannerapi.PodResourceInfo {
	scheduledPodNames := r.scheduledPodNamesByNodeName[nodeName].UnsortedList()
	if len(scheduledPodNames) == 0 {
		return nil
	}
	scheduledPodInfos := make([]plannerapi.PodResourceInfo, 0, len(scheduledPodNames))
	for _, podName := range scheduledPodNames {
		scheduledPodInfos = append(scheduledPodInfos, r.unscheduledPods[podName])
	}
	return scheduledPodInfos
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

// getScaleOutNodeAssignments gets the slice of [plannerapi.NodePodAssignment] to scale-out nodes of this simulation run.
func (r *RunState) getScaleOutNodeAssignments() (scaleOutAssignments []plannerapi.NodePodAssignment) {
	for name, node := range r.scaleOutNodes {
		scheduledPodInfos := r.getScheduledPodInfosForNode(name)
		if len(scheduledPodInfos) > 0 {
			nodeResources := getNodeResourceInfo(node)
			scaleOutAssignments = append(scaleOutAssignments, plannerapi.NodePodAssignment{
				NodeResources: nodeResources,
				ScheduledPods: scheduledPodInfos,
			})
		}
	}
	return
}

// getOtherPodNodeAssignments gets the slice of [plannerapi.NodePodAssignment] to nodes that were not scale-out nodes
// of this simulation run.
func (r *RunState) getOtherPodNodeAssignments() ([]plannerapi.NodePodAssignment, error) {
	assignedNodeNames := slices.Collect(maps.Keys(r.scheduledPodNamesByNodeName))
	assignedNodes, err := r.view.ListNodes(r.ctx, assignedNodeNames...)
	if err != nil {
		return nil, err
	}
	otherAssignments := make([]plannerapi.NodePodAssignment, len(assignedNodes))
	for _, node := range assignedNodes {
		_, isScaledOutNode := r.scaleOutNodes[node.Name]
		if isScaledOutNode {
			continue
		}
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

func getNodeResourceInfo(node *corev1.Node) plannerapi.NodeResourceInfo {
	instanceType := nodeutil.GetInstanceType(node)
	return plannerapi.NodeResourceInfo{
		Name:         node.Name,
		InstanceType: instanceType,
		Capacity:     node.Status.Capacity,
		Allocatable:  node.Status.Allocatable,
	}
}
