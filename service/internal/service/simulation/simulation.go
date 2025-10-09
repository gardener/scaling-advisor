// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package simulation

import (
	"context"
	"fmt"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"maps"
	"slices"
	"time"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

type defaultSimulation struct {
	name         string
	args         *svcapi.SimulationArgs
	nodeTemplate *sacorev1alpha1.NodeTemplate
	state        *trackState
}

var _ svcapi.CreateSimulationFunc = New

func New(name string, args *svcapi.SimulationArgs) (svcapi.Simulation, error) {
	var nodeTemplate *sacorev1alpha1.NodeTemplate
	for _, nt := range args.NodePool.NodeTemplates {
		if nt.Name == args.NodeTemplateName {
			nodeTemplate = &nt
			break
		}
	}
	if nodeTemplate == nil {
		return nil, fmt.Errorf("%w: node template %q not found in node pool %q", svcapi.ErrCreateSimulation, args.NodeTemplateName, args.NodePool.Name)
	}
	unscheduledPods, err := getUnscheduledPodsMap(args.View)
	if err != nil {
		return nil, fmt.Errorf("%w: simulation %q was unable to get unscheduled pods from view: %v", svcapi.ErrCreateSimulation, name, err)
	}
	if len(unscheduledPods) == 0 {
		return nil, fmt.Errorf("%w: %w: simulation %q was created with no unscheduled pods in its view", svcapi.ErrCreateSimulation, svcapi.ErrNoUnscheduledPods, name)
	}
	sim := &defaultSimulation{
		name:         name,
		args:         args,
		nodeTemplate: nodeTemplate,
		state: &trackState{
			status:              svcapi.ActivityStatusPending,
			unscheduledPods:     unscheduledPods,
			scheduledPodsByNode: make(map[string][]svcapi.PodResourceInfo),
		},
	}
	return sim, nil
}

func getUnscheduledPodsMap(v mkapi.View) (unscheduled map[types.NamespacedName]svcapi.PodResourceInfo, err error) {
	pods, err := v.ListPods(mkapi.MatchAllCriteria)
	if err != nil {
		return
	}
	unscheduled = make(map[types.NamespacedName]svcapi.PodResourceInfo, len(pods))
	for _, p := range pods {
		if IsUnscheduledPod(&p) {
			unscheduled[objutil.NamespacedName(&p)] = podutil.PodResourceInfoFromCoreV1Pod(&p)
		}
	}
	return
}

func getUnscheduledPods(v mkapi.View) (unscheduled []svcapi.PodResourceInfo, err error) {
	pods, err := v.ListPods(mkapi.MatchAllCriteria)
	if err != nil {
		return
	}
	unscheduled = make([]svcapi.PodResourceInfo, 0, len(pods))
	for _, p := range pods {
		if IsUnscheduledPod(&p) {
			unscheduled = append(unscheduled, podutil.PodResourceInfoFromCoreV1Pod(&p))
		}
	}
	return
}

func IsUnscheduledPod(pod *corev1.Pod) bool {
	return pod.Spec.NodeName == ""
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

func (s *defaultSimulation) ActivityStatus() svcapi.ActivityStatus {
	return s.state.status
}

func (s *defaultSimulation) Result() (svcapi.SimRunResult, error) {
	return s.state.result, s.state.err
}

func (s *defaultSimulation) Run(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: run of simulation %q failed: %w", svcapi.ErrRunSimulation, s.name, err)
			s.state.err = err
			s.state.status = svcapi.ActivityStatusFailure
		}
	}()
	s.state.status = svcapi.ActivityStatusRunning
	s.state.groupRunPassNum = s.args.GroupRunPassCounter.Load()
	s.state.simNode = s.buildSimulationNode()
	_, err = s.args.View.CreateObject(typeinfo.NodesDescriptor.GVK, s.state.simNode)
	if err != nil {
		return
	}
	simCtx, simCancelFn := newSimulationContext(ctx, s.name, s.args.Timeout)
	defer simCancelFn()

	schedulerHandle, err := s.launchSchedulerForSimulation(simCtx, s.args.View)
	if err != nil {
		return
	}
	defer schedulerHandle.Stop()

	err = s.trackUntilStabilized(simCtx)
	if err != nil {
		return
	}
	otherAssignments, err := s.getOtherAssignments()
	if err != nil {
		return
	}
	s.state.result = svcapi.SimRunResult{
		Name:       s.name,
		ScaledNode: s.state.simNode,
		NodeScorerArgs: svcapi.NodeScorerArgs{
			ID:               fmt.Sprintf("%s-%d", s.name, s.args.GroupRunPassCounter.Load()),
			Placement:        s.getScaledNodePlacementInfo(),
			ScaledAssignment: s.getScaledNodeAssignment(),
			UnscheduledPods:  slices.Collect(maps.Keys(s.state.unscheduledPods)),
			OtherAssignments: otherAssignments,
		},
	}
	s.state.status = svcapi.ActivityStatusSuccess
	return
}

func (s *defaultSimulation) getScaledNodePlacementInfo() sacorev1alpha1.NodePlacement {
	return sacorev1alpha1.NodePlacement{
		NodePoolName:     s.args.NodePool.Name,
		NodeTemplateName: s.nodeTemplate.Name,
		InstanceType:     s.nodeTemplate.InstanceType,
		AvailabilityZone: s.args.AvailabilityZone,
	}
}

func (s *defaultSimulation) getScaledNodeAssignment() *svcapi.NodePodAssignment {
	return &svcapi.NodePodAssignment{
		Node:          getNodeResourceInfo(s.state.simNode),
		ScheduledPods: s.state.scheduledPodsByNode[s.state.simNode.Name],
	}
}

func (s *defaultSimulation) launchSchedulerForSimulation(ctx context.Context, simView mkapi.View) (svcapi.SchedulerHandle, error) {
	clientFacades, err := simView.GetClientFacades(commontypes.ClientAccessInMemory)
	if err != nil {
		return nil, err
	}
	schedLaunchParams := &svcapi.SchedulerLaunchParams{
		ClientFacades: clientFacades,
		EventSink:     simView.GetEventSink(),
	}
	return s.args.SchedulerLauncher.Launch(ctx, schedLaunchParams)
}

func (s *defaultSimulation) buildSimulationNode() *corev1.Node {
	simNodeName := fmt.Sprintf("n-%d.%s.%s.%s", s.args.GroupRunPassCounter.Load(), s.args.AvailabilityZone, s.args.NodeTemplateName, s.args.NodePool.Name)
	nodeTaints := slices.Clone(s.args.NodePool.Taints)
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   simNodeName,
			Labels: nodeutil.CreateNodeLabels(s.name, s.args.NodePool, s.nodeTemplate, s.args.AvailabilityZone, s.args.GroupRunPassCounter.Load(), simNodeName),
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
func (s *defaultSimulation) trackUntilStabilized(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	v := s.args.View
	s.state.status = svcapi.ActivityStatusRunning
	var err error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err = s.state.reconcile(log, v, v.GetEventSink().List())
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

func (s *defaultSimulation) getOtherAssignments() ([]svcapi.NodePodAssignment, error) {
	nodeNames := slices.Collect(maps.Keys(s.state.scheduledPodsByNode))
	nodeNames = slices.DeleteFunc(nodeNames, func(nodeName string) bool {
		return nodeName == s.state.simNode.Name
	})
	nodes, err := s.args.View.ListNodes(nodeNames...)
	if err != nil {
		return nil, err
	}
	assignments := make([]svcapi.NodePodAssignment, 0, len(nodes))
	for _, node := range nodes {
		nodeResources := getNodeResourceInfo(&node)
		podResources := s.state.scheduledPodsByNode[node.Name]
		assignments = append(assignments, svcapi.NodePodAssignment{
			Node:          nodeResources,
			ScheduledPods: podResources,
		})
	}
	return assignments, nil
}

// traceState is regularly populated when simulation is running.
type trackState struct {
	groupRunPassNum     uint32
	status              svcapi.ActivityStatus
	simNode             *corev1.Node
	unscheduledPods     map[types.NamespacedName]svcapi.PodResourceInfo // map of Pod namespacedName to PodResourceInfo
	scheduledPodsByNode map[string][]svcapi.PodResourceInfo             // map of node names to PodReosurceInfo
	result              svcapi.SimRunResult
	err                 error
}

func (t *trackState) reconcile(log logr.Logger, view mkapi.View, events []eventsv1.Event) error {
	for _, ev := range events {
		log.V(4).Info("analyzing event", "ReportingController", ev.ReportingController, "ReportingInstance", ev.ReportingInstance, "Action", ev.Action, "Reason", ev.Reason, "Regarding", ev.Regarding)
		if ev.Action != "Binding" && ev.Reason != "Scheduled" {
			continue
		}
		podNsName := types.NamespacedName{Namespace: ev.Regarding.Namespace, Name: ev.Regarding.Name}
		log.Info("pod was scheduled", "namespacedName", podNsName, "eventNote", ev.Note)
		podObjName := cache.NamespacedNameAsObjectName(podNsName)
		obj, err := view.GetObject(typeinfo.PodsDescriptor.GVK, podObjName)
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
		found := slices.ContainsFunc(podsOnNode, func(podOnNode svcapi.PodResourceInfo) bool {
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

func newSimulationContext(ctx context.Context, simulationName string, timeout time.Duration) (context.Context, context.CancelFunc) {
	log := logr.FromContextOrDiscard(ctx)
	ctx = logr.NewContext(ctx, log.WithValues("simulationName", simulationName))
	ctx, cancel := context.WithTimeoutCause(ctx,
		timeout,
		fmt.Errorf("%w: %q timed out after duration %q", svcapi.ErrSimulationTimeout, simulationName, timeout))
	return ctx, cancel
}

func getNodeResourceInfo(node *corev1.Node) svcapi.NodeResourceInfo {
	instanceType := nodeutil.GetInstanceType(node)
	return svcapi.NodeResourceInfo{
		Name:         node.Name,
		InstanceType: instanceType,
		Capacity:     objutil.ResourceListToInt64Map(node.Status.Capacity),
		Allocatable:  objutil.ResourceListToInt64Map(node.Status.Allocatable),
	}
}
