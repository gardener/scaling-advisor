// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"io"
	"time"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	pricingapi "github.com/gardener/scaling-advisor/pricing/api"
	simulationapi "github.com/gardener/scaling-advisor/simulation/api"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RequestRef is the reference to a planner request.
type RequestRef struct {
	// ID is the Request unique identifier for which this response is generated.
	ID string `json:"id"`
	// CorrelationID is the correlation identifier for the request.
	// This can be used to correlate chains of requests and responses into a higher level activity.
	CorrelationID string `json:"correlationID,omitempty"`
}

// Request represents a request to the scaling planner to generate a scaling plan.
type Request struct {
	// CreationTime is the time at which request was created
	CreationTime time.Time `json:"creationTime,omitzero"`
	// Constraint represents the constraints using which the scaling advice is generated.
	Constraint *sacorev1alpha1.ScalingConstraint `json:"constraint,omitempty"`
	RequestRef
	// SimulatorStrategy defines the simulation strategy to be used for scaling virtual nodes for generation of scaling advice.
	SimulatorStrategy apicommon.SimulatorStrategy `json:"simulatorStrategy,omitempty"`
	// ScoringStrategy defines the node scoring strategy to use for scaling decisions.
	ScoringStrategy apicommon.NodeScoringStrategy `json:"scoringStrategy,omitempty"`
	// AdviceGenerationMode defines the mode in which scaling advice is generated.
	AdviceGenerationMode apicommon.ScalingAdviceGenerationMode `json:"adviceGenerationMode,omitempty"`
	// Snapshot is the snapshot of the resources in the cluster at the time of the request.
	Snapshot ClusterSnapshot `json:"snapshot,omitzero"`
	// AdviceGenerationTimeout is the maximum duration allowed for generating scaling advice.
	AdviceGenerationTimeout time.Duration `json:",omitzero"`
	// DiagnosticVerbosity indicates the level of diagnostics produced during scaling advice generation.
	// By default, its value is 0 that disables diagnostics.
	// The verbosity level is also passed to the logging framework (e.g. klog) used by scaling advisor components (e.g. kube-scheduler).
	DiagnosticVerbosity uint32 `json:"diagnosticVerbosity,omitzero"`
}

// GetRef returns the unique reference for the scaling advice request.
func (r *Request) GetRef() RequestRef {
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

// ClusterSnapshot represents a snapshot of the cluster at a specific time and encapsulates the scheduling relevant information required by the kube-scheduler.
// Pods inside the ClusterSnapshot should not have SchedulingGates - these should be filtered out by creator of the ClusterSnapshot.
type ClusterSnapshot struct {
	// Pods are the pods that are present in the cluster.
	Pods []PodInfo `json:"pods,omitempty"`
	// Nodes are the nodes that are present in the cluster.
	Nodes []NodeInfo `json:"nodes,omitempty"`
	// PVs are the information about PersistentVolumes in the cluster. Should not contain deleted PVs.
	// Should only contain *bound* PVs ie those with populated claimRef.
	PVs []PVInfo `json:"pvs,omitempty"`
	// PVCs are the information about PersistentVolumeClaims in the cluster. Should not contain deleted PVCs.
	PVCs []PVCInfo `json:"pvcs,omitempty"`
	//StorageClasses are the storage classes that are present in the cluster
	StorageClasses []storagev1.StorageClass `json:"storageClasses,omitempty"`
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

// PodInfo encapsulates only the necessary information about corev1.Pod that is required by the kube-scheduler.
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
	metav1.ObjectMeta `json:",inline"`
	// Volumes are the volumes that are attached to the Pod.
	Volumes []corev1.Volume `json:"volumes,omitempty"`
	// Tolerations are the tolerations for the Pod.
	Tolerations               []corev1.Toleration               `json:"tolerations,omitempty"`
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	ResourceClaims            []corev1.PodResourceClaim         `json:"resourceClaims,omitempty"`
	Priority                  int32                             `json:"priority,omitempty"`
}

// GetResourceInfo returns the resource information for the pod.
func (p *PodInfo) GetResourceInfo() simulationapi.PodResourceInfo {
	return simulationapi.PodResourceInfo{
		NamespacedName: apicommon.NamespacedName{
			Namespace: p.Namespace,
			Name:      p.Name,
		},
		AggregatedRequests: p.AggregatedRequests,
	}
}

// NodeInfo contains the minimum set of information about corev1.Node that will be required by the kube-scheduler.
type NodeInfo struct {
	// Capacity is the total resource capacity of the node.
	Capacity corev1.ResourceList `json:"capacity,omitempty"`
	// Allocatable is the allocatable resource capacity of the node.
	Allocatable corev1.ResourceList `json:"allocatable,omitempty"`
	// CSINodeSpec is the CSINodeSpec of the CSINode associated with this Node if any
	CSINodeSpec *storagev1.CSINodeSpec `json:"csiNodeSpec,omitempty"`
	// InstanceType is the instance type for the Node
	InstanceType      string `json:"instanceType"`
	metav1.ObjectMeta `json:",inline"`
	// Taints are the node's taints.
	Taints []corev1.Taint `json:"taints,omitempty"`
	// Conditions are the node's conditions.
	Conditions []corev1.NodeCondition `json:"conditions,omitempty"`
	// Unschedulable indicates whether the node is unschedulable.
	Unschedulable bool `json:"unschedulable,omitempty"`
}

// GetResourceInfo returns the resource information for the node.
func (n *NodeInfo) GetResourceInfo() simulationapi.NodeResourceInfo {
	return simulationapi.NodeResourceInfo{
		Name:         n.Name,
		InstanceType: n.InstanceType,
		Capacity:     n.Capacity,
		Allocatable:  n.Allocatable,
	}
}

// ValidateLabels validates that all required node labels are minimally present on this node info or returns an error wrapping the sentinel error
// apicommon.ErrMissingRequiredLabel
func (n *NodeInfo) ValidateLabels() error {
	for _, labelName := range RequiredNodeLabelNames.UnsortedList() {
		_, found := n.Labels[labelName]
		if !found {
			return fmt.Errorf("%w: missing %q in node %q", apicommon.ErrMissingRequiredLabel, labelName, n.Name)
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
		PoolName:         n.Labels[apicommon.LabelNodePoolName],
		TemplateName:     n.Labels[apicommon.LabelNodeTemplateName],
		InstanceType:     n.InstanceType,
		Region:           n.Labels[corev1.LabelTopologyRegion],
		AvailabilityZone: n.Labels[corev1.LabelTopologyZone],
	}
	return
}

// PVCInfo encapsulates the minimal set of scheduling relevant information about the k8s PersistentVolumeClaim.
type PVCInfo struct {
	corev1.PersistentVolumeClaimSpec `json:",inline"`
	Phase                            corev1.PersistentVolumeClaimPhase `json:"phase,omitempty"`
	metav1.ObjectMeta                `json:",inline"`
}

// PVInfo encapsulates the minimal set of scheduling relevant information about the k8s PersistentVolume.
type PVInfo struct {
	Capacity          corev1.ResourceList          `json:"capacity,omitempty"`
	NodeAffinity      *corev1.NodeSelector         `json:"nodeAffinity,omitzero"`
	ClaimRef          apicommon.NamespacedName     `json:"claimRef,omitzero"`
	StorageClassName  string                       `json:"storageClassName,omitempty"`
	Phase             corev1.PersistentVolumePhase `json:"phase,omitempty"`
	metav1.ObjectMeta `json:",inline"`
	VolumeMode        corev1.PersistentVolumeMode         `json:"volumeMode,omitempty"`
	AccessModes       []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

// GetNodeScorer is a factory function for creating NodeScorer implementations.
type GetNodeScorer func(scoringStrategy apicommon.NodeScoringStrategy, pricingAccess pricingapi.InstancePricingAccess, resourceWeigher ResourceWeigher) (NodeScorer, error)

// NodeScorer defines an interface for computing node scores for scaling decisions.
type NodeScorer interface {
	// Compute computes the node score given the NodeScorerArgs. On failure, it must return an error with the sentinel error api.ErrComputeNodeScore
	Compute(ctx context.Context, args NodeScorerArgs) (NodeScore, error)
	// Select selects the winning NodeScore amongst the NodeScores of a given simulation pass and returns the pointer to the same.
	// If there is no winning node score amongst the group, then it returns nil.
	Select(ctx context.Context, groupNodeScores []NodeScore) (winningNodeScore *NodeScore, err error)
}

// NodeScorerArgs contains arguments for node scoring computation.
type NodeScorerArgs struct {
	// ID that must be given to the NodeScore produced by the NodeScorer
	ID string
	// ScaledNodePlacement represents the placement information for the Node
	ScaledNodePlacement sacorev1alpha1.NodePlacement
	// ScaledNodePodAssignment represents the node-pod assignment of the scaled Node for the current run.
	ScaledNodePodAssignment *simulationapi.NodePodAssignment
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of the ClusterSnapshot,
	// or it is a winning simulated Node from a previous run.
	OtherNodePodAssignments []simulationapi.NodePodAssignment
	// LeftOverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after simulation is completed.
	LeftOverUnscheduledPods []apicommon.NamespacedName
}

// NodeScore represents the scoring result for a node in scaling simulations.
type NodeScore struct {
	ScaledNodeResource simulationapi.NodeResourceInfo
	// Placement represents the placement information for the Node.
	Placement sacorev1alpha1.NodePlacement
	// Name uniquely identifies this NodeScore
	Name            string
	UnscheduledPods []apicommon.NamespacedName
	// Value is the score value for this Node.
	Value int
}

// ResourceWeigher defines an interface for obtaining resource weights for scoring.
type ResourceWeigher interface {
	// GetWeights returns the resource weights for the given instance type.
	GetWeights(instanceType string) (map[corev1.ResourceName]float64, error)
}

// SimulatorConfig holds the configuration for the internal simulator used by the scaling advisor planner.
type SimulatorConfig struct {
	simulationapi.SimulationConfig
	// MaxParallelSimulations is the maximum number of parallel simulations that can be run by the scaling advisor planner.
	MaxParallelSimulations int
}

// ScalingPlannerArgs encapsulates the arguments required to create a ScalingPlanner.
type ScalingPlannerArgs struct {
	// ViewAccess provides access to the MinKAPI views.
	ViewAccess minkapi.ViewAccess
	// ResourceWeigher provides resource weights for scoring.
	ResourceWeigher ResourceWeigher
	// PricingAccess provides access to instance pricing information.
	PricingAccess pricingapi.InstancePricingAccess
	// StorageMetaAccess provides access to storage metadata.
	StorageMetaAccess simulationapi.StorageMetaAccess
	// SchedulerLauncher provides functionality to launch kube-scheduler instances.
	SchedulerLauncher simulationapi.SchedulerLauncher
	// SimulatorFactory is the factory facade to create simulators
	SimulatorFactory SimulatorFactory
	// SimulationFactory is the factory facade to create simulations.
	SimulationFactory simulationapi.SimulationFactory
	// TraceDir is the directory for storing traces when diagnostics are enabled.
	TraceDir string
	// SimulatorConfig holds the configuration for the internal simulator.
	SimulatorConfig SimulatorConfig
}

// ScalingPlanner defines the interface for computing scaling plans.
type ScalingPlanner interface {
	// Plan begins generation of scaling plans accepting a Request and returning a response channel
	// on which one or more planner Response is delivered.
	//
	// The channel will be closed when plan generation has completed, an error has occurred, context is canceled or
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

// ScalingPlannerFactory is a factory for ScalingPlanner's
type ScalingPlannerFactory interface {
	// NewPlanner accepts ScallingPlannerArgs and constructs a new ScalingPlanner.
	NewPlanner(args ScalingPlannerArgs) (ScalingPlanner, error)
}

// SimulatorFactory is a factory facade for constructing various kinds of simulators.
type SimulatorFactory interface {
	GetScaleOutSimulator(args SimulatorArgs) (ScaleOutSimulator, error)
	// TODO: Add GetScaleInSimulator here.
}

// SimulatorArgs is an encapsulation of the arguments used to create a ScaleOutSimulator or ScaleInSimulator.
// Not all the fields may be necessary for constructing a specific simulator implementation.
type SimulatorArgs struct {
	// ViewAccess holds the minkapi ViewAccess used to create views against which simulations are run.
	ViewAccess minkapi.ViewAccess
	// SchedulerLauncher holds the launched for the embedded kube-scheduler
	SchedulerLauncher simulationapi.SchedulerLauncher
	// StorageMetaAccess holds the access facade to storage metadata.
	StorageMetaAccess simulationapi.StorageMetaAccess
	// NodeScorer holds the facade to compute NodeScores for simulated scaled nodes.
	NodeScorer NodeScorer
	// Strategy holds the simulator strategy which customizes simulator implementation and behaviorchanges simulator implementation and behavior
	Strategy apicommon.SimulatorStrategy
	// TraceDir is the base directory for storing trace logs and other dump data by the simulator
	TraceDir string
	// Config holds the static simulator config parameters
	Config SimulatorConfig
}

// ScaleOutSimulator is a facade that executes [ScaleOutSimulation]'s to generate one or more [ScaleOutPlanResult]'s sent
// on a result channel.
// Implementations vary depending on the [apicommon.SimulatorStrategy] used.
//
// Depending upon the implementation creates and organizes [ScaleOutSimulation]'s into [ScaleOutSimGroup]'s differently.
//
//	ScalingConstraints (Legend: pa -> "Pool-A", "ta" -> "Node Template A", "zx" -> "Zone X")
//		{pa:1, {ta: 1, tb: 2, tc: 1}, {zx, zy}}
//		{pb:2, {tq: 2, tr: 1, ts: 1}, {zz}}
//	groups
//		g1: PriorityKey(2,2): [ {pb, tq, zz} ]
//		g2: PriorityKey(2,1): [ {pb, tr, zz}, {pb, ts, zz}]
//		g3: PriorityKey(1,2): [ {pa, tb, zx}, {pa, tb, zy}]
//		g1: PriorityKey(1,1): [ {pa, ta, zx}, {pa, ta, zy}, {pa, tc, zx}, {pa, tc, zy} ]
//
// An [ScaleOutSimulator] implementation created, will do the
// following when Simulate is invoked:
//   - Creates [ScaleOutSimulation]'s using the [SimulationFactory] given as parameter
//   - Organizes [ScaleOutSimulation]'s into [ScaleOutSimGroup]'s according to [PriorityKey] and [apicommon.SimulatorStrategy]
//   - Executes each ScaleOutSimGroup until stabilization, collecting ScaleOutSimResult's and aggregating them into SimulationGroupRunResult
//   - The SimulationGroupRunResult is c
//   - invoke the NodeScorer to determine a winner NodeScore
//
// ill run the different
// scale-out [simulation.Simulation]'s of a [simulation.SimulationGroup] concurrently where each simulation Run virtually
// scales ONE node in its MinKAPI overlay View for a [ScaleOutNodeTemplate] triple. The configured SchedulerLauncher
// is used to launch embedded `kube-scheduler` which does pod binding to the virtual scale-up node. This concludes one
// "Run" of the simulation.
//
// The scaled node which is the "winner" of this pass the simulator specific SimulatorStrategy to generate one or more
// ScaleOutPlan's each encapsulated within a ScaleOutPlanResult that is offered on the resultCh channel.
//
// Or may run a single simulation by scaling multiple nodes for a given group for all combinations of NodePool,
// NodeTemplate and AvailabilityZone. Simulations for a group are run before moving to the group at the next priority
// level. Moving to the next group is only done if there are leftover unscheduled pods after running all simulations in
// the current group.
type ScaleOutSimulator interface {
	io.Closer

	// Simulate is the high level activity that runs [ScaleOutSimulation] created from given
	// [SimulationFactory] with the given planner [Request].
	Simulate(ctx context.Context, request *Request, simulationFactory simulationapi.SimulationFactory) (planResult <-chan ScaleOutPlanResult)
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

// ScaleOutSimGroupPassScores represents the scoring results, including the winner score, for a single pass of a ScaleOutSimGroup
// after running the NodeScorer against the ScaleOutSimResult's of the pass.
type ScaleOutSimGroupPassScores struct {
	// WinnerScore is the highest scoring node in the group.
	WinnerScore *NodeScore
	// WinnerNode is the actual node corresponding to the winner score.
	WinnerNode *corev1.Node
	// AllScores contains all computed node scores for the group.
	AllScores []NodeScore
}

// ScaleOutSimGroupCycleResult represents the result of running all passes for a ScaleOutSimGroup.
type ScaleOutSimGroupCycleResult struct {
	// NextGroupPassView is the updated view after executing all passes in this group.
	// The next group, if any, should use this view as its base view for its overlay view.
	NextGroupPassView minkapi.View
	// Name is the name of the simulation group.
	Name string
	// WinnerNodeScores contains the node scores of the winning nodes.
	WinnerNodeScores []NodeScore
	// LeftoverUnscheduledPods contains the namespaced names of pods that could not be scheduled.
	LeftoverUnscheduledPods []apicommon.NamespacedName
	// PassNum is the number of passes executed in this group before moving to the next group.
	// A pass is defined as the execution of all simulations in a group.
	PassNum int
}

// ScalingPlannerService is the facade for the scaling planner microservice that embeds a ScalingPlanner
// Offers a REST API for the embedded ScalingPlanner
type ScalingPlannerService interface {
	apicommon.Service
	ScalingPlanner
}

// ScalingPlannerServiceConfig holds the service configuration for the scaling planner microservice.
type ScalingPlannerServiceConfig struct {
	// CloudProvider is the cloud provider for which the scaling advisor planner is initialized.
	CloudProvider apicommon.CloudProvider
	// TraceDir is the base directory for storing trace files produced by the scaling advisor planner.
	TraceDir string
	// ServerConfig holds the server configuration for the scaling advisor planner.
	ServerConfig apicommon.ServerConfig
	// MinKAPIConfig holds the configuration for the MinKAPI server used by the scaling advisor planner.
	MinKAPIConfig minkapi.Config
	// ClientConfig holds the client QPS and Burst settings for the scaling advisor planner.
	ClientConfig apicommon.QPSBurst
	// SimulatorConfig holds the configuration used by the internal simulator.
	SimulatorConfig SimulatorConfig
}

// Factories is a struct that holds all planner factories.
type Factories struct {
	Planner         ScalingPlannerFactory
	Simulator       SimulatorFactory
	Simulation      simulationapi.SimulationFactory
	ResourceWeigher ResourceWeigher
}
