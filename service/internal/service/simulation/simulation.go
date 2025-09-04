package simulation

import (
	"context"
	"fmt"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	mkapi "github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	"github.com/gardener/scaling-advisor/service/api"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"maps"
	"slices"
)

type defaultSimulation struct {
	name            string
	args            *api.SimulationArgs
	nodeTemplate    *sacorev1alpha1.NodeTemplate
	schedulerHandle api.SchedulerHandle
	state           *trackState
}

// traceState is regularly populated when simulation is running.
type trackState struct {
	status          api.ActivityStatus
	simNode         *corev1.Node
	unscheduledPods []api.PodResourceInfo
	scheduledPods   map[string][]api.PodResourceInfo
	result          api.SimRunResult
	err             error
}

var _ api.CreateSimulationFunc = New

func New(name string, args *api.SimulationArgs) (api.Simulation, error) {
	var nodeTemplate *sacorev1alpha1.NodeTemplate
	for _, nt := range args.NodePool.NodeTemplates {
		if nt.Name == args.NodeTemplateName {
			nodeTemplate = &nt
			break
		}
	}
	if nodeTemplate == nil {
		return nil, fmt.Errorf("%w: node template %q not found in node pool %q", api.ErrCreateSimulation, args.NodeTemplateName, args.NodePool.Name)
	}
	return &defaultSimulation{
		name:         name,
		args:         args,
		nodeTemplate: nodeTemplate,
		state: &trackState{
			status: api.ActivityStatusPending,
		},
	}, nil
}

func (s *defaultSimulation) NodePool() *sacorev1alpha1.NodePool {
	return s.args.NodePool
}
func (s *defaultSimulation) NodeTemplate() *sacorev1alpha1.NodeTemplate {
	return s.nodeTemplate
}

func (s *defaultSimulation) Name() string {
	return s.name
}
func (s *defaultSimulation) ActivityStatus() api.ActivityStatus {
	return s.state.status
}

func (s *defaultSimulation) Result() (api.SimRunResult, error) {
	return s.state.result, s.state.err
}

func (s *defaultSimulation) Run(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: run of simulation %q failed: %w", api.ErrRunSimulation, s.name, err)
			s.state.err = err
		}
	}()
	s.state.simNode = s.buildSimulationNode()
	err = s.args.View.StoreObject(typeinfo.NodesDescriptor.GVK, s.state.simNode)
	if err != nil {
		return
	}
	simCtx := newSimulationContext(ctx, s.name)
	schedulerHandle, err := s.launchSchedulerForSimulation(simCtx, s.args.View)
	if err != nil {
		return
	}
	s.schedulerHandle = schedulerHandle
	s.state.status = api.ActivityStatusRunning
	err = s.trackUntilStabilized(simCtx)
	if err != nil {
		return
	}
	assignments, err := s.getAssignments()
	if err != nil {
		return
	}
	s.state.result = api.SimRunResult{
		Name:       s.name,
		ScaledNode: s.state.simNode,
		NodeScoreArgs: api.NodeScoreArgs{
			Name:             s.name,
			Placement:        s.getScaledNodePlacementInfo(),
			ScaledAssignment: s.getScaledNodeAssignment(),
			UnscheduledPods:  s.state.unscheduledPods,
			Assignments:      assignments,
		},
	}
	return
}

func (s *defaultSimulation) getScaledNodePlacementInfo() api.NodePlacementInfo {
	return api.NodePlacementInfo{
		NodePoolName:     s.args.NodePool.Name,
		NodeTemplateName: s.nodeTemplate.Name,
		InstanceType:     s.nodeTemplate.InstanceType,
		AvailabilityZone: s.args.AvailabilityZone,
	}
}

func (s *defaultSimulation) getScaledNodeAssignment() *api.NodePodAssignment {
	return &api.NodePodAssignment{
		Node:          getNodeResourceInfo(s.state.simNode),
		ScheduledPods: s.state.scheduledPods[s.state.simNode.Name],
	}
}

func (s *defaultSimulation) launchSchedulerForSimulation(ctx context.Context, simView mkapi.View) (api.SchedulerHandle, error) {
	clientFacades, err := simView.GetClientFacades()
	if err != nil {
		return nil, err
	}
	schedLaunchParams := &api.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     simView.GetEventSink(),
	}
	return s.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

func (s *defaultSimulation) buildSimulationNode() *corev1.Node {
	/*
		create a simulation node based on the provided template, region, zone, labels, and taints.
		Add apiconstants.LabelSimulationID with the value of simulationName to the labels.
	*/
	return &corev1.Node{}
}

// trackUntilStabilized monitors the EventSink for scheduling events for all the unscheduled pods for a simulation run.
func (s *defaultSimulation) trackUntilStabilized(ctx context.Context) error {
	/*
			NOTE: If there is an error then you return the error.
			If the ctx.Done then return the ctx.Err
			Get all the unscheduled pods from the simulation view.
			This function starts a loop which does the following till one of the following conditions is met:
		      1. All the pods are scheduled within the stabilization period OR
		      2. Stabilization period is over and there are still unscheduled pods.

			At the end of the loop it does the following:
			1. Updates the defaultSimulation.state with unscheduled and scheduled pods.
	*/
	panic("implement me") //TODO immplement trackUntilStabilized
}

func (s *defaultSimulation) getAssignments() ([]api.NodePodAssignment, error) {
	nodeNames := slices.Collect(maps.Keys(s.state.scheduledPods))
	nodeNames = slices.DeleteFunc(nodeNames, func(nodeName string) bool {
		return nodeName == s.state.simNode.Name
	})
	nodes, err := s.args.View.ListNodes(nodeNames...)
	if err != nil {
		return nil, err
	}
	var assignments []api.NodePodAssignment
	for _, node := range nodes {
		nodeResources := getNodeResourceInfo(&node)
		podResources := s.state.scheduledPods[node.Name]
		assignments = append(assignments, api.NodePodAssignment{
			Node:          nodeResources,
			ScheduledPods: podResources,
		})
	}
	return assignments, nil
}

func newSimulationContext(ctx context.Context, simulationName string) context.Context {
	log := logr.FromContextOrDiscard(ctx)
	return logr.NewContext(ctx, log.WithValues("simulationName", simulationName))
}

func getNodeResourceInfo(node *corev1.Node) api.NodeResourceInfo {
	instanceType := nodeutil.GetInstanceType(node)
	return api.NodeResourceInfo{
		Name:         node.Name,
		InstanceType: instanceType,
		Capacity:     objutil.ResourceListToMapInt64(node.Status.Capacity),
		Allocatable:  objutil.ResourceListToMapInt64(node.Status.Allocatable),
	}
}
