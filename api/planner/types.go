// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/pricing"

	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
)

// ActivityStatus represents the operational status of an activity.
type ActivityStatus string

const (
	// ActivityStatusPending indicates the activity is pending execution.
	ActivityStatusPending ActivityStatus = "Pending"
	// ActivityStatusRunning indicates the activity is currently running.
	ActivityStatusRunning ActivityStatus = "Running"
	// ActivityStatusSuccess indicates the activity completed successfully.
	ActivityStatusSuccess ActivityStatus = metav1.StatusSuccess
	// ActivityStatusFailure indicates the activity failed.
	ActivityStatusFailure ActivityStatus = metav1.StatusFailure
)

// RequestRef is the reference to a planner request.
type RequestRef struct {
	// ID is the Request unique identifier for which this response is generated.
	ID string `json:"id"`
	// CorrelationID is the correlation identifier for the request.
	// This can be used to correlate chains of requests and responses into a higher level activity.
	CorrelationID string `json:"correlationID"`
}

// Request represents a request to the scaling planner to generate a scaling plan.
type Request struct {
	// CreationTime is the time at which request was created
	CreationTime time.Time `json:"creationTime,omitzero"`
	// Constraint represents the constraints using which the scaling advice is generated.
	Constraint *sacorev1alpha1.ScalingConstraint `json:"constraint,omitempty"`
	RequestRef
	// SimulatorStrategy defines the simulation strategy to be used for scaling virtual nodes for generation of scaling advice.
	SimulatorStrategy commontypes.SimulatorStrategy `json:"simulatorStrategy,omitempty"`
	// ScoringStrategy defines the node scoring strategy to use for scaling decisions.
	ScoringStrategy commontypes.NodeScoringStrategy `json:"scoringStrategy,omitempty"`
	// AdviceGenerationMode defines the mode in which scaling advice is generated.
	AdviceGenerationMode commontypes.ScalingAdviceGenerationMode `json:"adviceGenerationMode,omitempty"`
	// Snapshot is the snapshot of the resources in the cluster at the time of the request.
	Snapshot ClusterSnapshot `json:"snapshot,omitzero"`
	// AdviceGenerationTimeout is the maximum duration allowed for generating scaling advice.
	AdviceGenerationTimeout time.Duration `json:",omitzero"`
	// DiagnosticVerbosity indicates the level of  diagnostics produced during scaling advice generation.
	// By default, its value is 0 which disables diagnostics.
	// The verbosity level is also passed to the logging framework (e.g. klog) used by scaling advisor components (e.g. kube-scheduler).
	DiagnosticVerbosity uint32 `json:"diagnosticVerbosity,omitzero"`
}

// GetRef returns the unique reference for the scaling advice request.
func (r Request) GetRef() RequestRef {
	return RequestRef{
		ID:            r.ID,
		CorrelationID: r.CorrelationID,
	}
}

// Response represents the response from the scaling planner.
type Response struct {
	// RequestRef encapsulates the unique reference to a request for which this response is produced.
	RequestRef RequestRef
	// Error is any error encountered during plan generation. Represents a terminal error that occurred during plan generation
	// No further responses will be sent for the associated request.
	Error error `json:"error,omitempty"`
	// Labels is the associated metadata.
	Labels map[string]string `json:"labels,omitempty"`
	// ScaleOutPlan is the generated scale-out plan.
	ScaleOutPlan *sacorev1alpha1.ScaleOutPlan `json:"scaleOutPlan,omitempty"`
	// ScaleInPlan is the generated scale-in plan.
	ScaleInPlan *sacorev1alpha1.ScaleInPlan `json:"scaleInPlan,omitempty"`
	// ID is the identified for this response
	ID string `json:"id,omitempty"`
}

// SchedulerLaunchParams holds the parameters required to launch a kube-scheduler instance.
type SchedulerLaunchParams struct {
	// EventSink is the event sink used to send events from the kube-scheduler.
	EventSink events.EventSink
	commontypes.ClientFacades
}

// SchedulerLauncher defines the interface for launching a kube-scheduler instance.
// There will be a limited number of kube-scheduler instances that can be launched at a time.
type SchedulerLauncher interface {
	// Launch launches and runs an embedded scheduler instance asynchronously.
	// If the limit of running schedulers is reached, it will block.
	// An error is returned if the scheduler fails to start.
	Launch(ctx context.Context, params *SchedulerLaunchParams) (SchedulerHandle, error)
}

// SchedulerHandle defines the interface for managing a kube-scheduler instance.
type SchedulerHandle interface {
	io.Closer
	// GetParams returns the parameters used to launch the scheduler instance.
	GetParams() SchedulerLaunchParams
}

// ClusterSnapshot represents a snapshot of the cluster at a specific time and encapsulates the scheduling relevant information required by the kube-scheduler.
// Pods inside the ClusterSnapshot should not have SchedulingGates - these should be filtered out by creator of the ClusterSnapshot.
type ClusterSnapshot struct {
	// Pods are the pods that are present in the cluster.
	Pods []PodInfo `json:"pods,omitempty"`
	// Nodes are the nodes that are present in the cluster.
	Nodes []NodeInfo `json:"nodes,omitempty"`
	// PriorityClasses are the priority classes that are present in the cluster.
	PriorityClasses []schedulingv1.PriorityClass `json:"priorityClasses,omitempty"`
	// RuntimeClasses are the runtime classes that are present in the cluster.
	RuntimeClasses []nodev1.RuntimeClass `json:"runtimeClasses,omitempty"`
}

// GetUnscheduledPods returns all pods in the cluster snapshot that are not scheduled to any node.
func (c *ClusterSnapshot) GetUnscheduledPods() []PodInfo {
	var unscheduledPods []PodInfo
	for _, pod := range c.Pods {
		if pod.NodeName == "" {
			unscheduledPods = append(unscheduledPods, pod)
		}
	}
	return unscheduledPods
}

// GetNodeCountByPlacement returns a map of node placements to their respective node counts in the cluster.
func (c *ClusterSnapshot) GetNodeCountByPlacement() (map[sacorev1alpha1.NodePlacement]int32, error) {
	nodeCountByPlacement := make(map[sacorev1alpha1.NodePlacement]int32)
	for _, nodeInfo := range c.Nodes {
		p, err := nodeInfo.GetNodePlacement()
		if err != nil {
			return nil, err
		}
		nodeCountByPlacement[p]++
	}
	return nodeCountByPlacement, nil
}

// BasicMeta contains the basic metadata associated with Kubernetes resource objects that is relevant for the scaling planner	.
type BasicMeta struct {
	// UID is the unique identifier for the resource.
	UID       types.UID `json:"uid"`
	Namespace string    `json:"namespace,omitempty"`
	Name      string    `json:"name"`
	// Labels are the labels associated with the resource.
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations are the annotations associated with the resource.
	Annotations map[string]string `json:"annotations,omitempty"`
	// DeletionTimestamp is the timestamp when the resource deletion was triggered.
	DeletionTimestamp time.Time `json:"deletionTimestamp,omitzero"`
	// OwnerReferences are the owner references associated with the resource.
	OwnerReferences []metav1.OwnerReference `json:"ownerReferences,omitempty"`
}

// PodInfo contains the minimum set of information about corev1.Pod that will be required by the kube-scheduler.
// NOTES:
//  1. PodSchedulingGates should not be not part of PodInfo. It is expected that pods having scheduling gates will be filtered out before setting up simulation runs.
//  2. Consider including PodSpec.Resources in future when it graduates to beta/GA.
type PodInfo struct {
	// AggregatedRequests is an aggregated resource requests for all containers of the Pod.
	AggregatedRequests corev1.ResourceList `json:"aggregatedRequests,omitempty"`
	// NodeSelector is the node selector for the Pod.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Affinity is the affinity rules for the Pod.
	Affinity *corev1.Affinity    `json:"affinity,omitempty"`
	Overhead corev1.ResourceList `json:"overhead,omitempty"`
	// NodeName is the name of the node where the Pod is scheduled.
	NodeName string `json:"nodeName,omitempty"`
	// SchedulerName is the name of the scheduler that should be used to schedule the Pod.
	SchedulerName string `json:"schedulerName,omitempty"`
	// PriorityClassName is the name of the priority class that should be used to schedule the Pod.
	PriorityClassName string                  `json:"priorityClassName,omitempty"`
	PreemptionPolicy  corev1.PreemptionPolicy `json:"preemptionPolicy,omitempty"`
	RuntimeClassName  string                  `json:"runtimeClassName,omitempty"`
	BasicMeta
	// Volumes are the volumes that are attached to the Pod.
	Volumes []corev1.Volume `json:",omitempty"`
	// Tolerations are the tolerations for the Pod.
	Tolerations               []corev1.Toleration               `json:",omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:",omitempty"`
	ResourceClaims            []corev1.PodResourceClaim         `json:",omitempty"`
	Priority                  int32                             `json:",omitzero"`
}

// GetResourceInfo returns the resource information for the pod.
func (p *PodInfo) GetResourceInfo() PodResourceInfo {
	return PodResourceInfo{
		UID:                p.UID,
		Namespace:          p.Namespace,
		Name:               p.Name,
		AggregatedRequests: p.AggregatedRequests,
	}
}

// NodeInfo contains the minimum set of information about corev1.Node that will be required by the kube-scheduler.
type NodeInfo struct {
	// Capacity is the total resource capacity of the node.
	Capacity corev1.ResourceList `json:",omitempty"`
	// Allocatable is the allocatable resource capacity of the node.
	Allocatable corev1.ResourceList `json:",omitempty"`
	// CSIDriverVolumeMaximums is a map of CSI driver names to the maximum number of unique volumes managed by the
	// CSI driver that can be used on a node.
	CSIDriverVolumeMaximums map[string]int32 `json:",omitempty"`
	// InstanceType is the instance type for the Node
	InstanceType string
	BasicMeta
	// Taints are the node's taints.
	Taints []corev1.Taint `json:",omitempty"`
	// Conditions are the node's conditions.
	Conditions []corev1.NodeCondition `json:",omitempty"`
	// Unschedulable indicates whether the node is unschedulable.
	Unschedulable bool `json:",omitzero"`
}

// GetResourceInfo returns the resource information for the node.
func (n *NodeInfo) GetResourceInfo() NodeResourceInfo {
	return NodeResourceInfo{
		Name:         n.Name,
		InstanceType: n.InstanceType,
		Capacity:     n.Capacity,
		Allocatable:  n.Allocatable,
	}
}

// ValidateLabels validates that all required node labels are minimally present on this node info or returns an error wrapping the sentinel error
// commonconstants.ErrMissingRequiredLabel
func (n *NodeInfo) ValidateLabels() error {
	for _, labelName := range RequiredNodeLabelNames.UnsortedList() {
		_, found := n.Labels[labelName]
		if !found {
			return fmt.Errorf("%w: missing %q in node %q", commonerrors.ErrMissingRequiredLabel, labelName, n.Name)
		}
	}
	return nil
}

// GetNodePlacement extracts the node placement information from this NodeInfo.
func (n *NodeInfo) GetNodePlacement() (placement sacorev1alpha1.NodePlacement, err error) {
	err = n.ValidateLabels()
	if err != nil {
		return
	}
	placement = sacorev1alpha1.NodePlacement{
		NodePoolName:     n.Labels[commonconstants.LabelNodePoolName],
		NodeTemplateName: n.Labels[commonconstants.LabelNodeTemplateName],
		InstanceType:     n.InstanceType,
		Region:           n.Labels[corev1.LabelTopologyRegion],
		AvailabilityZone: n.Labels[corev1.LabelTopologyZone],
	}
	return
}

// GetNamespacedName returns the NamespacedName for this basic meta.
func (m *BasicMeta) GetNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.Namespace,
		Name:      m.Name,
	}
}

// GetNodeScorer is a factory function for creating NodeScorer implementations.
type GetNodeScorer func(scoringStrategy commontypes.NodeScoringStrategy, pricingAccess pricing.InstancePricingAccess, resourceWeigher ResourceWeigher) (NodeScorer, error)

// NodeScorer defines an interface for computing node scores for scaling decisions.
type NodeScorer interface {
	// Compute computes the node score given the NodeScorerArgs. On failure, it must return an error with the sentinel error api.ErrComputeNodeScore
	Compute(args NodeScorerArgs) (NodeScore, error)
	// Select selects the winning NodeScore amongst the NodeScores of a given simulation pass and returns the pointer to the same.
	// If there is no winning node score amongst the group, then it returns nil.
	Select(groupNodeScores []NodeScore) (winningNodeScore *NodeScore, err error)
}

// NodeScorerArgs contains arguments for node scoring computation.
type NodeScorerArgs struct {
	// ID that must be given to the NodeScore produced by the NodeScorer
	ID string
	// ScaledNodePlacement represents the placement information for the Node
	ScaledNodePlacement sacorev1alpha1.NodePlacement
	// ScaledNodePodAssignment represents the node-pod assignment of the scaled Node for the current run.
	ScaledNodePodAssignment *NodePodAssignment
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of the ClusterSnapshot
	// or it is a winning simulated Node from a previous run.
	OtherNodePodAssignments []NodePodAssignment
	// LeftOverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after simulation is completed.
	LeftOverUnscheduledPods []types.NamespacedName
}

// NodeScore represents the scoring result for a node in scaling simulations.
type NodeScore struct {
	ScaledNodeResource NodeResourceInfo
	// Placement represents the placement information for the Node.
	Placement sacorev1alpha1.NodePlacement
	// Name uniquely identifies this NodeScore
	Name            string
	UnscheduledPods []types.NamespacedName
	// Value is the score value for this Node.
	Value int
}

// GetResourceWeightsFunc is a function type for retrieving resource weights for scoring.
type GetResourceWeightsFunc func(instanceType string) (map[corev1.ResourceName]float64, error)

// GetWeights returns the resource weights for the given instance type.
func (f GetResourceWeightsFunc) GetWeights(instanceType string) (map[corev1.ResourceName]float64, error) {
	return f(instanceType)
}

// ResourceWeigher defines an interface for obtaining resource weights for scoring.
type ResourceWeigher interface {
	// GetWeights returns the resource weights for the given instance type.
	GetWeights(instanceType string) (map[corev1.ResourceName]float64, error)
}

// PodResourceInfo contains resource information for a pod used in scoring calculations.
type PodResourceInfo struct {
	// AggregatedRequests is an aggregated resource requests for all containers of the Pod.
	AggregatedRequests corev1.ResourceList `json:"aggregatedRequests,omitempty"`
	Namespace          string              `json:"namespace,omitempty"`
	Name               string              `json:"name"`
	// UID is the unique identifier for the pod.
	UID types.UID `json:"uid"`
}

// GetNamespacedName returns the NamespacedName for this PodResourceInfo.
func (m *PodResourceInfo) GetNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.Namespace,
		Name:      m.Name,
	}
}

// NodeResourceInfo represents the subset of NodeInfo such that NodeScorer can compute an effective NodeScore.
type NodeResourceInfo struct {
	// Capacity is the total resource capacity of the node.
	Capacity corev1.ResourceList
	// Allocatable is the allocatable resource capacity of the node.
	Allocatable corev1.ResourceList
	// Name is the node name.
	Name string
	// InstanceType is the cloud instance type of the node.
	InstanceType string
}

// NodePodAssignment represents the assignment of pods to a node for simulation purposes.
type NodePodAssignment struct {
	// NodeResources contains the resource information for the node.
	NodeResources NodeResourceInfo
	// ScheduledPods contains the list of pods scheduled to this node.
	ScheduledPods []PodResourceInfo
}

// SimulatorConfig holds the configuration for the internal simulator used by the scaling advisor planner.
type SimulatorConfig struct {
	// MaxParallelSimulations is the maximum number of parallel simulations that can be run by the scaling advisor planner.
	MaxParallelSimulations int
	// TrackPollInterval is the polling interval for tracking pod scheduling in the view of the simulator.
	TrackPollInterval time.Duration
}

// ScalingPlannerArgs encapsulates the arguments required to create a ScalingPlanner.
type ScalingPlannerArgs struct {
	// ViewAccess provides access to the MinKAPI views.
	ViewAccess minkapi.ViewAccess
	// ResourceWeigher provides resource weights for scoring.
	ResourceWeigher ResourceWeigher
	// PricingAccess provides access to instance pricing information.
	PricingAccess pricing.InstancePricingAccess
	// SchedulerLauncher provides functionality to launch kube-scheduler instances.
	SchedulerLauncher SchedulerLauncher
	// TraceLogsBaseDir is the base directory for storing trace logs when diagnostics are enabled for a scaling advice request.
	TraceLogsBaseDir string
	// SimulatorConfig holds the configuration for the internal simulator.
	SimulatorConfig SimulatorConfig
}

// ScalingPlanner defines the interface for computing scaling plans.
type ScalingPlanner interface {
	// Plan begins generation of scaling plans accepting a Request and returning a response channel
	// on which one or more planner Response is delivered.
	//
	// The channel will be closed when plan generation has completed, an error has occurred, orthe context is canceled or
	// timed-out.
	//
	// The caller must consume all Response's from the channel until it is closed to
	// avoid leaking goroutines inside the planner.
	//
	// The provided context can be used to cancel generation prematurely. In this
	// case, the channel will be closed without further events.
	//
	// Usage:
	//
	//	responseCh := planner.Plan(ctx, req)
	//	for r := range responseCh {
	//	    if r.Error != nil {
	//	        log.Printf("plan generation failed: %v", r.Error)
	//	        break
	//	    }
	//	    process(r.ScaleOutPlan)
	//	    process(r.ScaleInPlan)
	//	}
	Plan(ctx context.Context, req Request) <-chan Response
}

// ScaleOutSimulator is a facade that executes simulations to generate one or more scale-out plans.
// Implementations vary depending on the commontypes.SimulatorStrategy used.
//
// Depending upon the SimulatorStrategy, the implementation creates and organizes Simulation into SimulationGroup's differently.
//
//	(TODO: extend the example below with two availability zones - currently a single default one is assumed)
//
// SimulatorStrategySingleNodeMultiSim
//
//	ScalingConstraints
//		np-a: 1 {nt-a: 1, nt-b: 2, nt-c: 1}
//		np-b: 2 {nt-q: 2, nt-r: 1, nt-s: 1}
//	SimulationGroups
//		g1: {PoolPriority: 1, NTPriority: 1, nt-a, nt-c}
//		g2: {PoolPriority: 1, NTPriority: 2, nt-b}
//		g3: {PoolPriority: 2, NTPriority: 1, nt-r, nt-s}
//		g4: {PoolPriority: 2, NTPriority: 2, nt-q}
//
// SimulatorStrategyMultiNodeSingleSim
//
//	np-a: 1 {nt-a: 1, nt-b: 2, nt-c: 1}
//	np-b: 2 {nt-q: 2, nt-r: 1, nt-s: 1}
//	np-c: 1 {nt-x: 2, nt-y: 1}
//
//	g1: {PoolPriority: 1, NTPriority: 1, nt-a, nt-c, nt-y}
//	g2: {PoolPriority: 1, NTPriority: 2, nt-b, nt-x}
//	g3: {PoolPriority: 2, NTPriority: 1, nt-r, nt-s}
//	g4: {PoolPriority: 2, NTPriority: 2, nt-q}
//
// An implementation created for the SimulatorStrategySingleNodeMultiSim, will run the different Simulation's of a SimulationGroup
// concurrently where each simulation Run virtually scales ONE one node in its MinKAPI overlay View for a combination of NodePool, NodeTemplate and
// AvailabilityZone. The configured SchedulerLauncher is used to launch embedded Scheduler which does pod assignments to the virtual scaled node.
// This concludes one "Run" of the simulation.
// The scaled node which is the "winner" of this pass
//
// Or may run a single
// simulation by scaling
// multiple nodes for a given
// group for all combinations of NodePool, NodeTemplate and AvailabilityZone. Simulations for a group are run before moving
// to the group at the next priority level. Moving to the next group is only done if there are leftover unscheduled pods after
// running all simulations in the current group.
type ScaleOutSimulator interface {
	io.Closer
	// Simulate is the high level activity that
	//  - Creates Simulation's using the given SimulationCreator
	//  - Organizes Simulation's into SimulationGroup's according to priority and the configured SimulatorStrategy.
	//  - Executes each SimulationGroup until stabilization, collecting SimulationRunResult's and aggregating them into SimulationGroupRunResult
	//  - The SimulationGroupRunResult is c
	//  - invoke the NodeScorer to determine a winner NodeScore
	//
	// the simulator specific SimulatorStrategy to generate one or more ScaleOutPlan's each encapsulated within a
	// ScaleOutPlanResult that is offered on the resultCh channel.
	Simulate(ctx context.Context, request *Request, simulationCreator SimulationCreator) (planResult <-chan ScaleOutPlanResult)
}

// ScaleOutPlanResult represents a result from the ScaleOutSimulator.Simulate
type ScaleOutPlanResult struct {
	// Error is any error encountered during plan generation. Represents a terminal error that occurred during plan generation
	// No further responses will be sent for the associated request.
	Error error `json:"error,omitempty"`
	// Labels is the associated metadata.
	Labels map[string]string `json:"labels,omitempty"`
	// ScaleOutPlan is the generated scale-out plan.
	ScaleOutPlan *sacorev1alpha1.ScaleOutPlan `json:"scaleOutPlan,omitempty"`
}

// Simulation represents an activity that performs valid unscheduled pod to ready node assignments on a minkapi View.
// A simulation implementation may use a k8s scheduler - either embedded or external to do this, or it may form a SAT/MIP model
// from the pod/node data and run a tool that solves the model.
type Simulation interface {
	commontypes.Resettable
	// Name returns the logical simulation name
	Name() string
	// ActivityStatus returns the current ActivityStatus of the simulation
	ActivityStatus() ActivityStatus
	// NodePool returns the target node pool against which the simulation should be run
	NodePool() *sacorev1alpha1.NodePool
	// NodeTemplate returns the target node template against which the simulation should be run
	NodeTemplate() *sacorev1alpha1.NodeTemplate
	// Run executes the simulation against the given view to completion and returns any encountered error.
	// This is a blocking call, and callers are expected to manage concurrency and SimulationRunResult consumption.
	Run(ctx context.Context, view minkapi.View) error
	// Result returns the latest SimulationRunResult if the simulation is in ActivityStatusSuccess,
	// or nil if the simulation is in ActivityStatusPending or ActivityStatusRunning
	// or an error if the ActivityStatus is ActivityStatusFailure
	Result() (SimulationRunResult, error)
}

// SimulationRunResult contains the results of a completed simulation run.
type SimulationRunResult struct {
	// Name of the Simulation that produced this result.
	Name string
	// View is the minkapi View against which the simulation was run.
	View minkapi.View
	// ScaledNodePlacements represents the placement information for the scaled Nodes.
	ScaledNodePlacements []sacorev1alpha1.NodePlacement
	// ScaledNodePodAssignments represents the assignment of Pods to scaled Nodes.
	ScaledNodePodAssignments []NodePodAssignment
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of the ClusterSnapshot
	// or it is a winning simulated Node from a previous run.
	OtherNodePodAssignments []NodePodAssignment
	// LeftoverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after simulation is completed.
	LeftoverUnscheduledPods []types.NamespacedName
}

// SimulationArgs represents the argument necessary for creating a simulation instance.
type SimulationArgs struct {
	// SchedulerLauncher is used to launch scheduler instances for the simulation.
	SchedulerLauncher SchedulerLauncher
	// NodePool is the target node pool for the simulation.
	NodePool *sacorev1alpha1.NodePool
	// RunCounter is an atomic counter for tracking simulation runs.
	RunCounter *atomic.Uint32
	// AvailabilityZone is the target availability zone for the simulation.
	AvailabilityZone string
	// NodeTemplateName is the name of the node template to use in the simulation.
	NodeTemplateName string
	// Config is the simulation configuration.
	Config SimulatorConfig
}

// SimulationCreatorFunc is a factory function for constructing a simulation instance.
// It implements the SimulationCreator interface.
type SimulationCreatorFunc func(name string, args *SimulationArgs) (Simulation, error)

// SimulationCreator is an interface that wraps a method that satisfied SimulationCreatorFunc
type SimulationCreator interface {
	// Create creates a simulation instance with the given name and arguments.
	Create(name string, args *SimulationArgs) (Simulation, error)
}

// Create constructs a new simulation instance with the given name and arguments. Satisfies SimulationCreatorFunc
func (f SimulationCreatorFunc) Create(name string, args *SimulationArgs) (Simulation, error) {
	return f(name, args)
}

// GetSimulationViewFunc is a type alias for view provider functions that take a context and view name and return the associated minkapi View.
// Used to decouple components.
type GetSimulationViewFunc func(ctx context.Context, name string) (minkapi.View, error)

// SimulationGroup is a group of simulations at the same priority level (ie a partition of simulations).
type SimulationGroup interface {
	commontypes.Resettable
	// Name returns the name of the simulation group.
	Name() string
	// GetKey returns the simulation group key.
	GetKey() SimGroupKey
	// GetSimulations returns all simulations in this group.
	GetSimulations() []Simulation
	// AddSimulation adds a simulation to the group.
	AddSimulation(simulation Simulation)
	// Run executes all simulations in the group and returns all the simulation run results or any error.
	Run(ctx context.Context, getViewFn GetSimulationViewFunc) ([]SimulationRunResult, error)
}

// SimulationGrouperFunc represents a factory function for grouping Simulation instances into one or more SimulationGroups
type SimulationGrouperFunc func(simulations []Simulation) ([]SimulationGroup, error)

// SimulationGrouper is an interface that wraps a method that satisfies SimulationGrouperFunc
type SimulationGrouper interface {
	Group(simulations []Simulation) ([]SimulationGroup, error)
}

// Group groups Simulation instances into one or more SimulationGroups. Satisfies SimulationGrouperFunc
func (f SimulationGrouperFunc) Group(simulations []Simulation) ([]SimulationGroup, error) {
	return f(simulations)
}

// SimGroupKey represents the key for a SimulationGroup.
type SimGroupKey struct {
	// NodePoolPriority is the priority of the node pool.
	NodePoolPriority int32
	// NodeTemplatePriority is the priority of the node template.
	NodeTemplatePriority int32
}

// String returns a string representation of the SimGroupKey.
func (k SimGroupKey) String() string {
	return fmt.Sprintf("%d-%d", k.NodePoolPriority, k.NodeTemplatePriority)
}

// FIXME TODO: I don't think this is necessary.
//SimulationGroupRunResult contains the results of running a simulation group.
//type SimulationGroupRunResult struct {
//	// Name of the group that produced this result.
//	Name string
//	// SimulationResults contains the results from all simulations in the group.
//	SimulationResults []SimulationRunResult
//	// Key is the simulation group key (partition key)
//	Key SimGroupKey
//}

// SimulationGroupPassScores represents the scoring results, including the winner score, for a single pass of a SimulationGroup
// after running the NodeScorer against the SimulationRunResult's of the pass.
type SimulationGroupPassScores struct {
	// WinnerScore is the highest scoring node in the group.
	WinnerScore *NodeScore
	// WinnerNode is the actual node corresponding to the winner score.
	WinnerNode *corev1.Node
	// AllScores contains all computed node scores for the group.
	AllScores []NodeScore
}

// SimulationGroupCycleResult represents the result of running all passes for a SimulationGroup.
type SimulationGroupCycleResult struct {
	// CreatedAt is the time when this group run result was created.
	CreatedAt time.Time
	// NextGroupPassView is the updated view after executing all passes in this group.
	// The next group, if any, should use this view as its base view for its overlay view.
	NextGroupPassView minkapi.View
	// Name is the name of the simulation group.
	Name string
	// WinnerNodeScores contains the node scores of the winning nodes.
	WinnerNodeScores []NodeScore
	// LeftoverUnscheduledPods contains the namespaced names of pods that could not be scheduled.
	LeftoverUnscheduledPods []types.NamespacedName
	// PassNum is the number of passes executed in this group before moving to the next group.
	// A pass is defined as the execution of all simulations in a group.
	PassNum int
}

// ScalingPlannerService is the facade for the scaling planner microservice that embeds a ScalingPlanner
// Offers a REST API for the embedded ScalingPlanner
type ScalingPlannerService interface {
	commontypes.Service
	ScalingPlanner
}

// ScalingPlannerServiceConfig holds the service configuration for the scaling planner microservice.
type ScalingPlannerServiceConfig struct {
	// CloudProvider is the cloud provider for which the scaling advisor planner is initialized.
	CloudProvider commontypes.CloudProvider
	// TraceLogBaseDir is the base directory for storing trace log files used by the scaling advisor planner.
	TraceLogBaseDir string
	// ServerConfig holds the server configuration for the scaling advisor planner.
	ServerConfig commontypes.ServerConfig
	// MinKAPIConfig holds the configuration for the MinKAPI server used by the scaling advisor planner.
	MinKAPIConfig minkapi.Config
	// ClientConfig holds the client QPS and Burst settings for the scaling advisor planner.
	ClientConfig commontypes.QPSBurst
	// SimulatorConfig holds the configuration used by the internal simulator.
	SimulatorConfig SimulatorConfig
}
