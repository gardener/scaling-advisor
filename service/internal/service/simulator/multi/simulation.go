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
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/ioutil"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/cache"
)

var _ service.Simulation = (*singleNodeScalingSimulation)(nil)

type singleNodeScalingSimulation struct {
	args         *service.SimulationArgs
	nodeTemplate *sacorev1alpha1.NodeTemplate
	state        *trackState
	name         string
}

var _ service.SimulationCreatorFunc = NewSimulation

// NewSimulation creates a new Simulation instance with the specified name and using the given arguments after validation.
func NewSimulation(name string, args *service.SimulationArgs) (service.Simulation, error) {
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
		state: &trackState{
			status:              service.ActivityStatusPending,
			scheduledPodsByNode: make(map[string][]service.PodResourceInfo),
		},
	}
	return sim, nil
}

func (s *singleNodeScalingSimulation) Reset() {
	s.state = &trackState{
		status:              service.ActivityStatusPending,
		scheduledPodsByNode: make(map[string][]service.PodResourceInfo),
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

func (s *singleNodeScalingSimulation) ActivityStatus() service.ActivityStatus {
	return s.state.status
}

func (s *singleNodeScalingSimulation) Result() (service.SimulationResult, error) {
	return s.state.result, s.state.err
}

func (s *singleNodeScalingSimulation) Run(ctx context.Context, view minkapi.View) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: run of simulation %q failed: %w", service.ErrRunSimulation, s.name, err)
			s.state.err = err
			s.state.status = service.ActivityStatusFailure
		}
	}()

	log := logr.FromContextOrDiscard(ctx)
	simCtx := logr.NewContext(ctx, log.WithValues("simulationName", s.name))
	s.state.status = service.ActivityStatusRunning
	s.args.RunCounter.Add(1)

	// Get unscheduled pods from the view
	unscheduledPods, err := getUnscheduledPodsMap(simCtx, view)
	if err != nil {
		return fmt.Errorf("simulation %q was unable to get unscheduled pods from view %q: %w", s.name, view.GetName(), err)
	}
	if len(unscheduledPods) == 0 {
		return fmt.Errorf("%w: simulation %q was created with no unscheduled pods in the view %q", service.ErrNoUnscheduledPods, s.name, view.GetName())
	}
	s.state.unscheduledPods = unscheduledPods

	// Create simulation node
	s.state.simNode = s.buildSimulationNode()
	_, err = view.CreateObject(simCtx, typeinfo.NodesDescriptor.GVK, s.state.simNode)
	if err != nil {
		return
	}

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
	s.state.result = service.SimulationResult{
		Name:                     s.name,
		View:                     view,
		ScaledNodePlacements:     []sacorev1alpha1.NodePlacement{s.getScaledNodePlacementInfo()},
		ScaledNodePodAssignments: []service.NodePodAssignment{s.getScaledNodeAssignment()},
		OtherNodePodAssignments:  otherAssignments,
		LeftoverUnscheduledPods:  slices.Collect(maps.Keys(s.state.unscheduledPods)),
	}
	s.state.status = service.ActivityStatusSuccess
	return
}

func validateSimulationArgs(args *service.SimulationArgs, nodeTemplate *sacorev1alpha1.NodeTemplate) error {
	if nodeTemplate == nil {
		return fmt.Errorf("%w: node template %q not found in node pool %q", service.ErrCreateSimulation, args.NodeTemplateName, args.NodePool.Name)
	}
	if args.NodePool == nil {
		return fmt.Errorf("%w: node pool must not be nil", service.ErrCreateSimulation)
	}
	errList := sacorev1alpha1.ValidateNodePool(args.NodePool, field.NewPath("nodePool"))
	if len(errList) > 0 {
		return fmt.Errorf("%w: invalid node pool %q: %v", service.ErrCreateSimulation, args.NodePool.Name, errList.ToAggregate())
	}
	if args.TrackPollInterval <= 0 {
		return fmt.Errorf("%w: track poll interval must be positive duration", service.ErrCreateSimulation)
	}
	if args.SchedulerLauncher == nil {
		return fmt.Errorf("%w: scheduler launcher must not be nil", service.ErrCreateSimulation)
	}
	return nil
}

func getUnscheduledPodsMap(ctx context.Context, v minkapi.View) (unscheduled map[types.NamespacedName]service.PodResourceInfo, err error) {
	pods, err := v.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return
	}
	unscheduled = make(map[types.NamespacedName]service.PodResourceInfo, len(pods))
	for _, p := range pods {
		if IsUnscheduledPod(&p) {
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

func (s *singleNodeScalingSimulation) getScaledNodeAssignment() service.NodePodAssignment {
	return service.NodePodAssignment{
		NodeResources: getNodeResourceInfo(s.state.simNode),
		ScheduledPods: s.state.scheduledPodsByNode[s.state.simNode.Name],
	}
}

func (s *singleNodeScalingSimulation) launchSchedulerForSimulation(ctx context.Context, simView minkapi.View) (service.SchedulerHandle, error) {
	clientFacades, err := simView.GetClientFacades(ctx, commontypes.ClientAccessModeInMemory)
	if err != nil {
		return nil, err
	}
	schedLaunchParams := &service.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     simView.GetEventSink(),
	}
	return s.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

func (s *singleNodeScalingSimulation) buildSimulationNode() *corev1.Node {
	simNodeName := fmt.Sprintf("n-%d.%s.%s.%s", s.args.RunCounter.Load(), s.args.AvailabilityZone, s.args.NodeTemplateName, s.args.NodePool.Name)
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
			Allocatable: nodeutil.ComputeAllocatable(s.nodeTemplate.Capacity, s.nodeTemplate.SystemReserved, s.nodeTemplate.SystemReserved),
			Conditions:  nodeutil.BuildReadyConditions(time.Now()),
		},
	}
}

// trackUntilStabilized starts a loop which updates the track state of the simulation until one of the following conditions is met:
//  1. All the pods are scheduled within the stabilization period OR
//  2. Stabilization period is over and there are still unscheduled pods.
func (s *singleNodeScalingSimulation) trackUntilStabilized(ctx context.Context, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	s.state.status = service.ActivityStatusRunning
	var err error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err = s.state.reconcile(ctx, view)
			if err != nil {
				return err
			}
			if len(s.state.unscheduledPods) == 0 {
				log.Info("no unscheduled pods left")
				return nil
			}
		}
		<-time.After(s.args.TrackPollInterval)
	}
}

func (s *singleNodeScalingSimulation) getOtherAssignments(ctx context.Context, view minkapi.View) ([]service.NodePodAssignment, error) {
	nodeNames := slices.Collect(maps.Keys(s.state.scheduledPodsByNode))
	nodeNames = slices.DeleteFunc(nodeNames, func(nodeName string) bool {
		return nodeName == s.state.simNode.Name
	})
	nodes, err := view.ListNodes(ctx, nodeNames...)
	if err != nil {
		return nil, err
	}
	assignments := make([]service.NodePodAssignment, 0, len(nodes))
	for _, node := range nodes {
		nodeResources := getNodeResourceInfo(&node)
		podResources := s.state.scheduledPodsByNode[node.Name]
		assignments = append(assignments, service.NodePodAssignment{
			NodeResources: nodeResources,
			ScheduledPods: podResources,
		})
	}
	return assignments, nil
}

// trackState is regularly populated when simulation is running.
type trackState struct {
	err                 error
	simNode             *corev1.Node
	unscheduledPods     map[types.NamespacedName]service.PodResourceInfo // map of Pod namespacedName to PodResourceInfo
	scheduledPodsByNode map[string][]service.PodResourceInfo             // map of node names to PodReosurceInfo
	status              service.ActivityStatus
	result              service.SimulationResult
}

func (t *trackState) reconcile(ctx context.Context, view minkapi.View) error {
	log := logr.FromContextOrDiscard(ctx)
	for _, ev := range view.GetEventSink().List() {
		log.V(4).Info("analyzing event", "ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance, "Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding)
		if ev.Action != "Binding" && ev.Reason != "Scheduled" {
			continue
		}
		podNsName := types.NamespacedName{Namespace: ev.Regarding.Namespace, Name: ev.Regarding.Name}
		log.Info("pod was scheduled", "namespacedName", podNsName, "eventNote", ev.Note)
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
			return fmt.Errorf("pod %q has no assigned node name even with binding event note %q", podNsName, ev.Note)
		}
		podsOnNode := t.scheduledPodsByNode[pod.Spec.NodeName]
		found := slices.ContainsFunc(podsOnNode, func(podOnNode service.PodResourceInfo) bool {
			return podOnNode.NamespacedName == podNsName
		})
		if found {
			continue
		}
		podsOnNode = append(podsOnNode, podutil.PodResourceInfoFromCoreV1Pod(pod))
		t.scheduledPodsByNode[pod.Spec.NodeName] = podsOnNode
		log.V(4).Info("pod added to trackState.scheduledPodsByNode", "namespacedName", podNsName, "nodeName", pod.Spec.NodeName, "numScheduledPods", len(t.scheduledPodsByNode))
		delete(t.unscheduledPods, podNsName)
	}
	return nil
}

func getNodeResourceInfo(node *corev1.Node) service.NodeResourceInfo {
	instanceType := nodeutil.GetInstanceType(node)
	return service.NodeResourceInfo{
		Name:         node.Name,
		InstanceType: instanceType,
		Capacity:     objutil.ResourceListToInt64Map(node.Status.Capacity),
		Allocatable:  objutil.ResourceListToInt64Map(node.Status.Allocatable),
	}
}
