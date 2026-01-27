// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package multi

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/logutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/cache"
)

var _ planner.Simulation = (*singleNodeScalingSimulation)(nil)

type singleNodeScalingSimulation struct {
	args         *planner.SimulationArgs
	nodeTemplate *sacorev1alpha1.NodeTemplate
	state        *runState
	name         string
}

var _ planner.SimulationCreatorFunc = NewSimulation

// NewSimulation creates a new Simulation instance with the specified name and using the given arguments after validation.
func NewSimulation(name string, args *planner.SimulationArgs) (planner.Simulation, error) {
	var nodeTemplate *sacorev1alpha1.NodeTemplate
	for _, nt := range args.NodePool.NodeTemplates {
		if nt.Name == args.NodeTemplateName {
			nodeTemplate = &nt
			break
		}
	}
	if err := validateSimulationArgs(args, nodeTemplate); err != nil {
		return nil, err
	}
	sim := &singleNodeScalingSimulation{
		name:         name,
		args:         args,
		nodeTemplate: nodeTemplate,
		state: &runState{
			status:              planner.ActivityStatusPending,
			scheduledPodsByNode: make(map[string][]planner.PodResourceInfo),
		},
	}
	return sim, nil
}

func (s *singleNodeScalingSimulation) Reset() {
	s.state = &runState{
		status:              planner.ActivityStatusPending,
		scheduledPodsByNode: make(map[string][]planner.PodResourceInfo),
	}
}

// IsUnscheduledPod determines if a given pod is unscheduled by checking if the NodeName in its spec is empty.
func IsUnscheduledPod(pod *corev1.Pod) bool {
	return pod.Spec.NodeName == ""
}

func (s *singleNodeScalingSimulation) NodePool() *sacorev1alpha1.NodePool {
	return s.args.NodePool
}

func (s *singleNodeScalingSimulation) NodeTemplate() *sacorev1alpha1.NodeTemplate {
	return s.nodeTemplate
}

func (s *singleNodeScalingSimulation) Name() string {
	return s.name
}

func (s *singleNodeScalingSimulation) ActivityStatus() planner.ActivityStatus {
	return s.state.status
}

func (s *singleNodeScalingSimulation) Result() (planner.SimulationResult, error) {
	return s.state.result, s.state.err
}

func (s *singleNodeScalingSimulation) Run(ctx context.Context, view minkapi.View) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: run of simulation %q failed: %w", planner.ErrRunSimulation, s.name, err)
			s.state.err = err
			s.state.status = planner.ActivityStatusFailure
		}
	}()

	log := logr.FromContextOrDiscard(ctx)
	simCtx := logr.NewContext(ctx, log.WithValues("simulationName", s.name))
	s.state.status = planner.ActivityStatusRunning
	s.args.RunCounter.Add(1)

	// Get unscheduled pods from the view
	unscheduledPods, err := getUnscheduledPodsMap(simCtx, view)
	if err != nil {
		return fmt.Errorf("simulation %q was unable to get unscheduled pods from view %q: %w", s.name, view.GetName(), err)
	}
	if len(unscheduledPods) == 0 {
		return fmt.Errorf("%w: simulation %q was created with no unscheduled pods in the view %q", planner.ErrNoUnscheduledPods, s.name, view.GetName())
	}
	s.state.unscheduledPods = unscheduledPods

	// Create simulation node
	s.state.simNode = s.buildSimulationNode()
	_, err = view.CreateObject(simCtx, typeinfo.NodesDescriptor.GVK, s.state.simNode)
	if err != nil {
		return
	}

	if logutil.VerbosityFromContext(ctx) > 1 {
		log.Info("created simulation node",
			"nodeName", s.state.simNode.Name,
			"instanceType", s.state.simNode.Labels[corev1.LabelInstanceTypeStable],
			"capacity", s.state.simNode.Status.Capacity,
			"allocatable", s.state.simNode.Status.Allocatable,
			"numUnscheduledPods", len(s.state.unscheduledPods))
	}
	simCtx = logr.NewContext(ctx, log.WithValues("simNodeName", s.state.simNode.Name))
	// Launch scheduler to operate on the simulation view and wait until stabilization
	schedulerHandle, err := s.launchSchedulerForSimulation(simCtx, view)
	if err != nil {
		return
	}
	defer ioutil.CloseQuietly(schedulerHandle)
	err = s.trackUntilStabilized(simCtx, view)
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
	s.state.result = planner.SimulationResult{
		Name:                     s.name,
		View:                     view,
		ScaledNodes:              []*corev1.Node{s.state.simNode},
		ScaledNodePlacements:     []sacorev1alpha1.NodePlacement{s.getScaledNodePlacementInfo()},
		ScaledNodePodAssignments: []planner.NodePodAssignment{s.getScaledNodeAssignment()},
		OtherNodePodAssignments:  otherAssignments,
		LeftoverUnscheduledPods:  slices.Collect(maps.Keys(s.state.unscheduledPods)),
	}
	s.state.status = planner.ActivityStatusSuccess
	return
}

func validateSimulationArgs(args *planner.SimulationArgs, nodeTemplate *sacorev1alpha1.NodeTemplate) error {
	if nodeTemplate == nil {
		return fmt.Errorf("%w: node template %q not found in node pool %q", planner.ErrCreateSimulation, args.NodeTemplateName, args.NodePool.Name)
	}
	if args.NodePool == nil {
		return fmt.Errorf("%w: node pool must not be nil", planner.ErrCreateSimulation)
	}
	errList := sacorev1alpha1.ValidateNodePool(args.NodePool, field.NewPath("nodePool"))
	if len(errList) > 0 {
		return fmt.Errorf("%w: invalid node pool %q: %v", planner.ErrCreateSimulation, args.NodePool.Name, errList.ToAggregate())
	}
	if args.Config.TrackPollInterval <= 0 {
		return fmt.Errorf("%w: track poll interval must be positive duration", planner.ErrCreateSimulation)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("%w: scheduler launcher must not be nil", planner.ErrCreateSimulation)
	}
	return nil
}

func getUnscheduledPodsMap(ctx context.Context, v minkapi.View) (unscheduled map[types.NamespacedName]planner.PodResourceInfo, err error) {
	log := logr.FromContextOrDiscard(ctx)
	pods, err := v.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return
	}
	unscheduled = make(map[types.NamespacedName]planner.PodResourceInfo, len(pods))
	for _, p := range pods {
		if IsUnscheduledPod(&p) {
			if logutil.VerbosityFromContext(ctx) > 2 {
				log.Info("found unscheduled pod", "pod", p)
			}
			unscheduled[objutil.NamespacedName(&p)] = podutil.PodResourceInfoFromCoreV1Pod(&p)
		}
	}
	return
}

func (s *singleNodeScalingSimulation) getScaledNodePlacementInfo() sacorev1alpha1.NodePlacement {
	return sacorev1alpha1.NodePlacement{
		NodePoolName:     s.args.NodePool.Name,
		NodeTemplateName: s.nodeTemplate.Name,
		InstanceType:     s.nodeTemplate.InstanceType,
		AvailabilityZone: s.args.AvailabilityZone,
		Region:           s.args.NodePool.Region,
	}
}

func (s *singleNodeScalingSimulation) getScaledNodeAssignment() planner.NodePodAssignment {
	return planner.NodePodAssignment{
		NodeResources: getNodeResourceInfo(s.state.simNode),
		ScheduledPods: s.state.scheduledPodsByNode[s.state.simNode.Name],
	}
}

func (s *singleNodeScalingSimulation) launchSchedulerForSimulation(ctx context.Context, simView minkapi.View) (planner.SchedulerHandle, error) {
	clientFacades, err := simView.GetClientFacades(ctx, commontypes.ClientAccessModeInMemory)
	if err != nil {
		return nil, err
	}
	schedLaunchParams := &planner.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     simView.GetEventSink(),
	}
	return s.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

func (s *singleNodeScalingSimulation) buildSimulationNode() *corev1.Node {
	simNodeName := fmt.Sprintf("node-%s.%s.%s.%d", s.args.NodePool.Name, s.args.NodeTemplateName, s.args.AvailabilityZone, s.args.RunCounter.Load())
	nodeTaints := slices.Clone(s.args.NodePool.Taints)
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   simNodeName,
			Labels: nodeutil.CreateNodeLabels(s.name, s.args.NodePool, s.nodeTemplate, s.args.AvailabilityZone, s.args.RunCounter.Load(), simNodeName),
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

const maxUnchangedReconciles = 2

// trackUntilStabilized starts a loop which updates the state of the simulation until one of the following conditions is met:
//  1. All the pods are scheduled within the stabilization period OR
//  2. Stabilization period is over and there are still unscheduled pods.
func (s *singleNodeScalingSimulation) trackUntilStabilized(ctx context.Context, view minkapi.View) (err error) {
	log := logr.FromContextOrDiscard(ctx)
	s.state.status = planner.ActivityStatusRunning
	var stabilized bool
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			<-time.After(s.args.Config.TrackPollInterval)
			stabilized, err = s.track(ctx, view)
			if err != nil {
				return
			}
			if stabilized {
				log.Info("simulation has stabilized - no more scheduling possible in this simulation run")
				return
			}
			if len(s.state.unscheduledPods) == 0 {
				log.Info("no unscheduled pods left")
				return
			}
		}
	}
}

func (s *singleNodeScalingSimulation) getOtherAssignments(ctx context.Context, view minkapi.View) ([]planner.NodePodAssignment, error) {
	nodeNames := slices.Collect(maps.Keys(s.state.scheduledPodsByNode))
	nodes, err := view.ListNodes(ctx, nodeNames...)
	if err != nil {
		return nil, err
	}
	var assignments []planner.NodePodAssignment
	for _, node := range nodes {
		if node.Name == s.state.simNode.Name {
			continue
		}
		podResources := s.state.scheduledPodsByNode[node.Name]
		if len(podResources) > 0 {
			nodeResources := getNodeResourceInfo(&node)
			assignments = append(assignments, planner.NodePodAssignment{
				NodeResources: nodeResources,
				ScheduledPods: podResources,
			})
		}
	}
	return assignments, nil
}

func (s *singleNodeScalingSimulation) track(ctx context.Context, view minkapi.View) (stabilized bool, err error) {
	var (
		eventTime metav1.MicroTime
	)
	s.state.numInvokedReconciles++

	lastRecordedReconcileEventTime := s.state.latestReconcileEventTime

	log := logr.FromContextOrDiscard(ctx)
	evList := view.GetEventSink().List()
	log.Info("track Invoked", "numEvents", len(evList), "numInvokedReconciles", s.state.numInvokedReconciles, "numUnchangedReconciles", s.state.numUnchangedReconciles)
	for idx, ev := range view.GetEventSink().List() {
		if ev.Series != nil {
			eventTime = ev.Series.LastObservedTime
		} else {
			eventTime = ev.EventTime
		}
		log.V(5).Info("checking event", "index", idx, "id", ev.UID, "eventTime", eventTime, "ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance, "Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding, "Note", ev.Note)
		if s.state.latestReconcileEventTime.Equal(&eventTime) || s.state.latestReconcileEventTime.After(eventTime.Time) {
			continue
		}
		s.state.latestReconcileEventTime = eventTime
		if ev.Action != "Binding" && ev.Reason != "Scheduled" {
			if ev.Reason == "FailedScheduling" {
				log.Info("failed scheduling event", "index", idx, "id", ev.UID, "ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance, "Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding, "Note", ev.Note)
			}
			continue
		}
		log.Info("scheduled event", "index", idx, "id", ev.UID, "ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance, "Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding, "Note", ev.Note)
		if err = s.handleScheduledPodEvent(ctx, view, ev); err != nil {
			return
		}
	}

	nodes, _ := view.ListNodes(ctx)
	pods, _ := view.ListPods(ctx, minkapi.MatchAllCriteria)
	log.Info("podNodeCount", "podCount", len(pods), "nodeCount", len(nodes))

	if lastRecordedReconcileEventTime.Equal(&s.state.latestReconcileEventTime) {
		s.state.numUnchangedReconciles++
	}
	if s.state.numUnchangedReconciles >= maxUnchangedReconciles {
		log.Info("no new scheduling events observed", "numUnchangedReconciles", s.state.numUnchangedReconciles, "lastRecordedReconcileEventTime", s.state.latestReconcileEventTime)
		stabilized = true
		if s.state.numScheduledPods == 0 {
			err = fmt.Errorf("%w: No scheduledPod for simulationNode %q or any one of %d existing node(s)", planner.ErrSimulationStabilizedWithNoScheduledPods, s.state.simNode)
		}
	}

	return
}

func (s *singleNodeScalingSimulation) handleScheduledPodEvent(ctx context.Context, view minkapi.View, ev eventsv1.Event) error {
	log := logr.FromContextOrDiscard(ctx)
	podNsName := types.NamespacedName{Namespace: ev.Regarding.Namespace, Name: ev.Regarding.Name}
	log.Info("scheduledPod event.", "namespacedName", podNsName, "eventNote", ev.Note)
	podObjName := cache.NamespacedNameAsObjectName(podNsName)
	obj, err := view.GetObject(ctx, typeinfo.PodsDescriptor.GVK, podObjName)
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
	podsOnNode := s.state.scheduledPodsByNode[pod.Spec.NodeName]
	found := slices.ContainsFunc(podsOnNode, func(podOnNode planner.PodResourceInfo) bool {
		return podOnNode.NamespacedName == podNsName
	})
	if found {
		return nil
	}
	podsOnNode = append(podsOnNode, podutil.PodResourceInfoFromCoreV1Pod(pod))
	s.state.scheduledPodsByNode[pod.Spec.NodeName] = podsOnNode
	s.state.numScheduledPods++
	log.V(4).Info("scheduledPod added to runState.scheduledPodsByNode", "namespacedName", podNsName, "nodeName", pod.Spec.NodeName, "numScheduledPods", s.state.numScheduledPods)
	delete(s.state.unscheduledPods, podNsName)
	return nil
}

func getNodeResourceInfo(node *corev1.Node) planner.NodeResourceInfo {
	instanceType := nodeutil.GetInstanceType(node)
	return planner.NodeResourceInfo{
		Name:         node.Name,
		InstanceType: instanceType,
		Capacity:     objutil.ResourceListToInt64Map(node.Status.Capacity),
		Allocatable:  objutil.ResourceListToInt64Map(node.Status.Allocatable),
	}
}

// runState is an internal state struct encapsulating details of parent singleNodeScalingSimulation.Run() and is updated when singleNodeScalingSimulation.track is invoked regularly by singleNodeScalingSimulation.trackUntilStabilized.
type runState struct {
	latestReconcileEventTime metav1.MicroTime
	err                      error
	simNode                  *corev1.Node
	unscheduledPods          map[types.NamespacedName]planner.PodResourceInfo // map of Pod namespacedName to PodResourceInfo
	scheduledPodsByNode      map[string][]planner.PodResourceInfo             // map of node names to PodReosurceInfo
	status                   planner.ActivityStatus
	result                   planner.SimulationResult
	numUnchangedReconciles   int
	numInvokedReconciles     int
	numScheduledPods         int
}
