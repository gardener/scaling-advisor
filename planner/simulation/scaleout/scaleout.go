// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scaleout

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/gardener/scaling-advisor/planner/util"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var _ plannerapi.ScaleOutSimulation = (*defaultScaleOut)(nil)

// defaultScaleOut is the default implementation of a ScaleOutSimulation.
type defaultScaleOut struct {
	args         *plannerapi.ScaleOutSimArgs
	nodeTemplate *sacorev1alpha1.NodeTemplate //TODO: change this to []nodeTemplate
	state        *runState
	name         string
	priorityKey  plannerapi.PriorityKey
}

// NewDefault creates a new ScaleOutSimulation instance with the specified name and using the given arguments after validation.
func NewDefault(name string, args plannerapi.ScaleOutSimArgs) (plannerapi.ScaleOutSimulation, error) {
	var nodeTemplate *sacorev1alpha1.NodeTemplate
	for _, nt := range args.NodePool.NodeTemplates {
		if nt.Name == args.NodeTemplateName {
			nodeTemplate = &nt
			break
		}
	}
	if err := validateSimArgs(&args, nodeTemplate); err != nil {
		return nil, err
	}

	sim := &defaultScaleOut{
		name:         name,
		args:         &args,
		nodeTemplate: nodeTemplate,
		priorityKey:  plannerapi.PriorityKey{NodePoolPriority: args.NodePool.Priority, NodeTemplatePriority: nodeTemplate.Priority},
		state: &runState{
			status:                      plannerapi.ActivityStatusPending,
			scheduledPodNamesByNodeName: make(map[string]sets.Set[commontypes.NamespacedName]),
		},
	}
	return sim, nil
}

func (s *defaultScaleOut) Reset() error {
	s.state = &runState{
		status:                      plannerapi.ActivityStatusPending,
		scheduledPodNamesByNodeName: make(map[string]sets.Set[commontypes.NamespacedName]),
	}
	return nil
}

func (s *defaultScaleOut) PriorityKey() plannerapi.PriorityKey {
	return s.priorityKey
}

func (s *defaultScaleOut) Name() string {
	return s.name
}

func (s *defaultScaleOut) ActivityStatus() plannerapi.ActivityStatus {
	return s.state.status
}

func (s *defaultScaleOut) Result() (plannerapi.ScaleOutSimResult, error) {
	return s.state.result, s.state.err
}

func (s *defaultScaleOut) Run(ctx context.Context, view minkapi.View) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: cannot run %q, runNum %d: %w", plannerapi.ErrRunSimulation, s.name, s.runNum(), err)
			s.state.err = err
			s.state.status = plannerapi.ActivityStatusFailure
		}
	}()
	s.state.status = plannerapi.ActivityStatusRunning
	runNum := s.incRunNum()
	log := logr.FromContextOrDiscard(ctx).WithValues("simulationName", s.name, "runNum", runNum)
	simCtx := logr.NewContext(ctx, log)

	if logutil.VerbosityFromContext(simCtx) > 3 {
		_ = viewutil.LogDumpObjects(simCtx, "SIMULATION-VIEW_BEFORE-RUN", view)
	}

	// Get unscheduled pods from the view
	unscheduledPods, err := getUnscheduledPodsMap(simCtx, view)
	if err != nil {
		return fmt.Errorf("simulation %q, runNum %d was unable to get unscheduled pods from view %q: %w",
			s.name, runNum, view.GetName(), err)
	}
	if len(unscheduledPods) == 0 {
		return fmt.Errorf("%w: simulation %q, runNum %d was created with no unscheduled pods in the view %q",
			plannerapi.ErrNoUnscheduledPods, s.name, runNum, view.GetName())
	}
	s.state.unscheduledPods = unscheduledPods
	s.state.leftoverUnscheduledPodNames = sets.New(slices.Collect(maps.Keys(unscheduledPods))...)

	// Create simulation Node and CSINode
	if err = s.createSimulationNode(simCtx, view); err != nil {
		return
	}
	simCtx = logr.NewContext(simCtx, log.WithValues("simNodeName", s.state.simNode.Name))
	if err = s.createCSINode(simCtx, s.state.simNode, view); err != nil {
		return
	}

	// Run static PVC<->PV Binding
	if _, err = util.BindClaimsAndVolumesForImmediateMode(simCtx, view); err != nil {
		return
	}

	// Launch scheduler to operate on the simulation view and wait until stabilization
	schedulerHandle, err := s.launchSchedulerForSimulation(simCtx, view)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(schedulerHandle)

	err = s.workAndTrackUntilStabilized(simCtx, view)
	if err != nil {
		return
	}

	// check for assignments done to either nodes that are part of the cluster snapshot or to nodes that are winners
	// from the previous runs.
	otherAssignments, err := s.getOtherAssignments(simCtx, view)
	if err != nil {
		return
	}

	// create simulation result
	s.state.result = plannerapi.ScaleOutSimResult{
		Name:                     s.name,
		View:                     view,
		ScaledNodePlacements:     []sacorev1alpha1.NodePlacement{s.getScaledNodePlacementInfo()},
		ScaledNodePodAssignments: s.getScaledNodeAssignments(),
		OtherNodePodAssignments:  otherAssignments,
		LeftoverUnscheduledPods:  s.state.leftoverUnscheduledPodNames.UnsortedList(),
	}
	s.state.status = plannerapi.ActivityStatusSuccess
	if len(s.state.result.LeftoverUnscheduledPods) > 0 {
		log.V(3).Info("LeftoverUnscheduledPods after run", "podCount", len(s.state.result.LeftoverUnscheduledPods))
	}
	return
}

func validateSimArgs(args *plannerapi.ScaleOutSimArgs, nodeTemplate *sacorev1alpha1.NodeTemplate) error {
	if nodeTemplate == nil {
		return fmt.Errorf("%w: node template %q not found in node pool %q", plannerapi.ErrCreateSimulation, args.NodeTemplateName, args.NodePool.Name)
	}
	if args.NodePool == nil {
		return fmt.Errorf("%w: node pool must not be nil", plannerapi.ErrCreateSimulation)
	}
	errList := sacorev1alpha1.ValidateNodePool(args.NodePool, field.NewPath("nodePool"))
	if len(errList) > 0 {
		return fmt.Errorf("%w: invalid node pool %q: %v", plannerapi.ErrCreateSimulation, args.NodePool.Name, errList.ToAggregate())
	}
	if args.Config.TrackPollInterval <= 0 {
		return fmt.Errorf("%w: track poll interval must be positive duration", plannerapi.ErrCreateSimulation)
	}
	if args.Config.MaxUnchangedTrackAttempts <= 0 {
		return fmt.Errorf("%w: max unchanged track attempts must be positive", plannerapi.ErrCreateSimulation)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("%w: scheduler launcher must not be nil", plannerapi.ErrCreateSimulation)
	}
	return nil
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

func (s *defaultScaleOut) getScaledNodePlacementInfo() sacorev1alpha1.NodePlacement {
	return sacorev1alpha1.NodePlacement{
		NodePoolName:     s.args.NodePool.Name,
		NodeTemplateName: s.nodeTemplate.Name,
		InstanceType:     s.nodeTemplate.InstanceType,
		AvailabilityZone: s.args.AvailabilityZone,
		Region:           s.args.NodePool.Region,
	}
}

func (s *defaultScaleOut) getScaledNodeAssignments() []plannerapi.NodePodAssignment {
	simNodeScheduledPods := s.state.scheduledPodNamesByNodeName[s.state.simNode.Name]
	if len(simNodeScheduledPods) == 0 {
		return nil
	}
	scheduledPodNames := s.state.scheduledPodNamesByNodeName[s.state.simNode.Name].UnsortedList()
	scheduledPodInfos := make([]plannerapi.PodResourceInfo, 0, len(scheduledPodNames))
	for _, podName := range scheduledPodNames {
		scheduledPodInfos = append(scheduledPodInfos, s.state.unscheduledPods[podName])
	}
	return []plannerapi.NodePodAssignment{
		{
			NodeResources: getNodeResourceInfo(s.state.simNode),
			ScheduledPods: scheduledPodInfos,
		},
	}
}

func (s *defaultScaleOut) launchSchedulerForSimulation(ctx context.Context, simView minkapi.View) (plannerapi.SchedulerHandle, error) {
	clientFacades, err := simView.GetClientFacades(ctx, commontypes.ClientAccessModeInMemory)
	if err != nil {
		return nil, err
	}
	schedLaunchParams := &plannerapi.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     simView.GetEventSink(),
	}
	return s.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

func (s *defaultScaleOut) createSimulationNode(ctx context.Context, view minkapi.View) error {
	simNode := s.buildSimulationNode()
	simNodeRuntimeObj, err := view.CreateObject(ctx, typeinfo.NodesDescriptor.GVK, simNode)
	if err != nil {
		return err
	}
	s.state.simNode = simNodeRuntimeObj.(*corev1.Node)
	log := logr.FromContextOrDiscard(ctx)
	log.V(2).Info("created simulation scaled node",
		"simNodeName", s.state.simNode.Name,
		"UID", s.state.simNode.UID,
		"instanceType", s.state.simNode.Labels[corev1.LabelInstanceTypeStable],
		"region", s.state.simNode.Labels[corev1.LabelTopologyRegion],
		"availabilityZone", s.state.simNode.Labels[corev1.LabelTopologyZone],
		"capacity", s.state.simNode.Status.Capacity,
		"allocatable", s.state.simNode.Status.Allocatable,
		"numUnscheduledPods", len(s.state.unscheduledPods))
	if logutil.VerbosityFromContext(ctx) >= viewutil.DefaultDumpVerbosity {
		simNodeDumpPath, err := objutil.SaveRuntimeObjAsYAMLToPath(s.state.simNode, s.args.TraceDir, s.state.simNode.Name+".yaml")
		if err != nil {
			return err
		}
		log.V(viewutil.DefaultDumpVerbosity).Info("dumped simulation node YAML", "simNodeName", simNode.Name, "simNodeDumpPath", simNodeDumpPath)
	}
	return nil
}

func (s *defaultScaleOut) createCSINode(ctx context.Context, node *corev1.Node, view minkapi.View) error {
	csiNodeSpec, err := s.args.StorageMetaAccess.GetFallbackCSINodeSpec(node.Labels[corev1.LabelInstanceTypeStable])
	if err != nil {
		return err
	}
	for i := range csiNodeSpec.Drivers {
		csiNodeSpec.Drivers[i].NodeID = node.Name
	}
	runtimeObj, err := view.CreateObject(ctx, typeinfo.CSINodeDescriptor.GVK, nodeutil.NewCSINode(node.Name, node.UID, csiNodeSpec))
	if err != nil {
		return err
	}
	csiNodeObj := runtimeObj.(*storagev1.CSINode)
	log := logr.FromContextOrDiscard(ctx)
	log.V(4).Info("created CSINode for scaled node", "name", csiNodeObj.GetName(), "ownerReferences", csiNodeObj.GetOwnerReferences())
	if logutil.VerbosityFromContext(ctx) >= viewutil.DefaultDumpVerbosity {
		csiNodeDumpPath, err := objutil.SaveRuntimeObjAsYAMLToPath(csiNodeObj, s.args.TraceDir, "csi-"+s.state.simNode.Name+".yaml")
		if err != nil {
			return err
		}
		log.V(viewutil.DefaultDumpVerbosity).Info("dumped CSINode YAML", "csiNodeName", csiNodeObj.Name, "csiNodeDumpPath", csiNodeDumpPath)
	}
	return err
}

func (s *defaultScaleOut) buildSimulationNode() *corev1.Node {
	simNodeName := fmt.Sprintf("simNode-%d_%s_%s_%s", s.runNum(), s.args.NodePool.Name, s.args.NodeTemplateName, s.args.AvailabilityZone)
	nodeTaints := slices.Clone(s.args.NodePool.Taints)
	nodeLabels := make(map[string]string)
	nodeLabels[commonconstants.LabelSimulationRunNum] = fmt.Sprintf("%d", s.runNum())
	nodeutil.AddNodeLabels(nodeLabels, s.nodeTemplate.Architecture, simNodeName, sacorev1alpha1.NodePlacement{
		NodePoolName:     s.args.NodePool.Name,
		NodeTemplateName: s.args.NodeTemplateName,
		InstanceType:     s.nodeTemplate.InstanceType,
		Region:           s.args.NodePool.Region,
		AvailabilityZone: s.args.AvailabilityZone,
	})
	nodeLabels["topology.ebs.csi.aws.com/zone"] = s.args.AvailabilityZone // TODO: need this for edge cases
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   simNodeName,
			Labels: nodeLabels,
		},
		Spec: corev1.NodeSpec{
			ProviderID: simNodeName,
			Taints:     nodeTaints,
		},
		Status: corev1.NodeStatus{
			Capacity:    s.nodeTemplate.Capacity,
			Allocatable: nodeutil.BuildAllocatable(s.nodeTemplate.Capacity, s.nodeTemplate.SystemReserved, s.nodeTemplate.KubeReserved),
			Conditions:  nodeutil.BuildReadyConditions(time.Now()),
		},
	}
}

// workAndTrackUntilStabilized starts a loop which performs work and tracks the state of the simulation until one of the following conditions is met:
//  1. All the pods are scheduled.
//  2. Events have stabilized. i.e., no more scheduling events within maxUnchangedTrackAttempts
//  3. Context timeout.
//  4. Any error
func (s *defaultScaleOut) workAndTrackUntilStabilized(ctx context.Context, view minkapi.View) (err error) {
	log := logr.FromContextOrDiscard(ctx)
	var stabilized bool
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			if err = s.doWork(ctx, view); err != nil {
				return
			}
			<-time.After(s.args.Config.TrackPollInterval)
			if stabilized, err = s.track(ctx, view); err != nil || stabilized {
				return
			}
			if len(s.state.leftoverUnscheduledPodNames) == 0 {
				log.V(2).Info("ending simulation run since leftoverUnscheduledPodNames is zero", "numTrackAttempts", s.state.numTrackAttempts)
				return
			}
		}
	}
}

func (s *defaultScaleOut) getOtherAssignments(ctx context.Context, view minkapi.View) ([]plannerapi.NodePodAssignment, error) {
	nodeNames := slices.Collect(maps.Keys(s.state.scheduledPodNamesByNodeName))
	nodes, err := view.ListNodes(ctx, nodeNames...)
	if err != nil {
		return nil, err
	}
	var assignments []plannerapi.NodePodAssignment
	for _, node := range nodes {
		if node.Name == s.state.simNode.Name {
			continue
		}
		scheduledPodInfos := s.state.scheduledPodInfos()
		if len(scheduledPodInfos) > 0 {
			nodeResources := getNodeResourceInfo(&node)
			assignments = append(assignments, plannerapi.NodePodAssignment{
				NodeResources: nodeResources,
				ScheduledPods: scheduledPodInfos,
			})
		}
	}
	return assignments, nil
}

func (s *defaultScaleOut) track(ctx context.Context, view minkapi.View) (stabilized bool, err error) {
	var eventTime metav1.MicroTime
	s.state.numTrackAttempts++
	evList := view.GetEventSink().List()
	if err = view.GetEventSink().Reset(); err != nil {
		return
	}
	log := logr.FromContextOrDiscard(ctx)
	log.V(4).Info("Invoked track", "numEvents", len(evList), "numTrackAttempts", s.state.numTrackAttempts, "numUnchangedTrackAttempts", s.state.numUnchangedTrackAttempts)
	for idx, ev := range evList {
		if ev.Series != nil {
			eventTime = ev.Series.LastObservedTime
		} else {
			eventTime = ev.EventTime
		}
		log.V(5).Info("Checking event", "index", idx, "id", ev.UID, "eventTime", eventTime, "ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance, "Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding, "Note", ev.Note)
		s.state.numReceivedEvents++
		if ev.Action != "Binding" && ev.Reason != "Scheduled" {
			if ev.Reason == "FailedScheduling" {
				log.V(4).Info("FailedScheduling event", "index", idx, "id", ev.UID, "ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance, "Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding, "Note", ev.Note)
			}
			continue
		}
		if err = s.state.handleScheduledPodEvent(ctx, view, ev); err != nil {
			return
		}
	}

	if len(evList) == 0 {
		s.state.numUnchangedTrackAttempts++
	}

	if s.state.numUnchangedTrackAttempts >= s.args.Config.MaxUnchangedTrackAttempts {
		log.V(3).Info("simulation run stabilized - no new events observed",
			"numReceivedEvents", s.state.numReceivedEvents,
			"maxUnchangedTrackAttempts", s.args.Config.MaxUnchangedTrackAttempts,
			"numUnchangedTrackAttempts", s.state.numUnchangedTrackAttempts,
			"numScheduledPods", s.state.numScheduledPods)
		stabilized = true
	}
	return
}

func (s *defaultScaleOut) runNum() uint32 {
	return s.args.RunCounter.Load()
}

func (s *defaultScaleOut) incRunNum() uint32 {
	return s.args.RunCounter.Add(1)
}

// doWork does miscellaneous simulation work to ensure that the kube-scheduler can
// continue pod-node bindings. Currently, it only delegates to
// util.BindClaimsAndVolumesWithNonNilClaimRefs, but other reconcile logic is likely to be incorporated in the future.
func (s *defaultScaleOut) doWork(ctx context.Context, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(3).Info("Invoked doWork", "viewName", view.GetName())
	numBound, err := util.BindClaimsAndVolumesWithNonNilClaimRefs(ctx, view)
	if err != nil {
		return err
	}
	if numBound > 0 {
		log.V(3).Info("Reset numUnchangedTrackAttempts since BindClaimsAndVolumesWithNonNilClaimRefs performed work", "numBound", numBound)
		// reset track state
		s.state.numUnchangedTrackAttempts = 0
	}
	return nil
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

// runState is an internal state struct encapsulating details of parent singleNodeScalingSimulation.Run() and is updated when defaultScaleOut.track is invoked regularly by singleNodeScalingSimulation.workAndTrackUntilStabilized.
type runState struct {
	err                         error
	simNode                     *corev1.Node
	unscheduledPods             map[commontypes.NamespacedName]plannerapi.PodResourceInfo // map of unscheduled Pod namespacedName to PodResourceInfo
	scheduledPodNamesByNodeName map[string]sets.Set[commontypes.NamespacedName]           // map of node names to set of scheduled pod names
	leftoverUnscheduledPodNames sets.Set[commontypes.NamespacedName]                      // represents a set of pod names scheduled during simulation run
	status                      plannerapi.ActivityStatus
	result                      plannerapi.ScaleOutSimResult
	numUnchangedTrackAttempts   int
	numTrackAttempts            int
	numReceivedEvents           int
	numScheduledPods            int
}

func (s *runState) scheduledPodInfos() []plannerapi.PodResourceInfo {
	scheduledPodNames := s.scheduledPodNamesByNodeName[s.simNode.Name].UnsortedList()
	scheduledPodInfos := make([]plannerapi.PodResourceInfo, 0, len(scheduledPodNames))
	for _, podName := range scheduledPodNames {
		scheduledPodInfos = append(scheduledPodInfos, s.unscheduledPods[podName])
	}
	return scheduledPodInfos
}

func (s *runState) handleScheduledPodEvent(ctx context.Context, view minkapi.View, ev eventsv1.Event) error {
	log := logr.FromContextOrDiscard(ctx)
	podNsName := objutil.NamespacedNameFromEventRegarding(ev)
	log.V(4).Info("PodScheduled event.", "podNamespacedName", podNsName, "eventNote", ev.Note)
	obj, err := view.GetObject(ctx, typeinfo.PodsDescriptor.GVK, podNsName.AsObjectName())
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
	s.addScheduledPod(ctx, pod)
	return nil
}

func (s *runState) addScheduledPod(ctx context.Context, pod *corev1.Pod) {
	log := logr.FromContextOrDiscard(ctx)
	podNsName := objutil.NamespacedName(pod)
	scheduledPodNames := s.scheduledPodNamesByNodeName[s.simNode.Name]
	if scheduledPodNames == nil {
		scheduledPodNames = sets.New[commontypes.NamespacedName]()
	}
	scheduledPodNames.Insert(podNsName)
	s.scheduledPodNamesByNodeName[pod.Spec.NodeName] = scheduledPodNames
	s.numScheduledPods++
	s.leftoverUnscheduledPodNames.Delete(podNsName)
	log.V(4).Info("Added scheduledPod to simulation.state.scheduledPodNamesByNodeName",
		"podNamespacedName", podNsName,
		"numScheduledPods", s.numScheduledPods,
		"leftoverUnscheduledPodCount", len(s.leftoverUnscheduledPodNames))
}
