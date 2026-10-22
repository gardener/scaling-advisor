// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"io"
	"time"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	apicomponent "github.com/gardener/scaling-advisor/api/common/component"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
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
	// CreationTime is the time at which the request was created.
	CreationTime time.Time `json:"creationTime,omitzero"`
	// Memento carries cross-invocation state from the previous planner response, enabling incremental planning.
	Memento Memento `json:"memento,omitempty"`
	// ScalingConstraint defines the node pools and strategies used to generate scaling advice.
	ScalingConstraint *sacorev1alpha1.ScalingConstraintSpec `json:"constraint,omitempty"`
	// NodeTemplates is a map of template name to [sacorev1alpha1.NodeTemplate], covering all templates
	// referenced by node pools in [Request.ScalingConstraint].
	NodeTemplates map[string]sacorev1alpha1.NodeTemplate `json:"nodeTemplates,omitempty"`
	// InstanceTypeDefs is a map of instance type name to [sacorev1alpha1.InstanceTypeDef], covering all
	// instance types referenced by [Request.NodeTemplates].
	InstanceTypeDefs map[string]sacorev1alpha1.InstanceTypeDef `json:"instanceTypeDefs,omitempty"`
	// InstancePrices lists pricing data for all instance types declared in [Request.NodeTemplates].
	// Used by [apicommon.NodeScoringStrategyLeastCost] in the [NodeScorer].
	InstancePrices []sacorev1alpha1.InstancePrice `json:"instancePrices,omitempty"`
	// ResourceWeights maps resource names (e.g. cpu, memory) to their relative weights for node scoring.
	// If nil, the scorer uses its built-in defaults.
	ResourceWeights map[corev1.ResourceName]float64 `json:"resourceWeights,omitempty"`
	RequestRef
	// AdviceGenerationMode controls whether scaling advice is generated incrementally or all-at-once.
	AdviceGenerationMode apicommon.ScalingAdviceGenerationMode `json:"adviceGenerationMode,omitempty"`
	// ClusterSnapshot is the scheduling-relevant cluster state at the time of the request.
	ClusterSnapshot ClusterSnapshot `json:"clusterSnapshot,omitzero"`
	// AdviceGenerationTimeout is the maximum duration allowed for generating scaling advice. Zero means no timeout.
	AdviceGenerationTimeout time.Duration `json:"adviceGenerationTimeout,omitzero"`
	// DiagnosticVerbosity controls the level of diagnostics produced during advice generation.
	// Zero disables diagnostics; higher values increase verbosity passed to internal components (e.g. kube-scheduler).
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
	RequestRef RequestRef `json:"requestRef"`
	// Error is any error encountered during plan generation. Represents a terminal error that occurred during plan generation.
	// No further responses will be sent by the [ScalingPlanner] for the associated request.
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

// ClusterSnapshot represents a snapshot of the cluster at a specific time and encapsulates the scheduling relevant
// information required by the kube-scheduler.
// Pods inside the ClusterSnapshot should not have SchedulingGates - these should be filtered out by creator of the ClusterSnapshot.
type ClusterSnapshot struct {
	// FallbackCSINodeSpecs provides CSINodeSpec data for nodes that do not have an associated CSINode object. Keyed by instanceType.
	FallbackCSINodeSpecs map[string]storagev1.CSINodeSpec `json:"fallbackCSINodeSpecs,omitempty"`
	// Pods are all pods in the cluster, both scheduled and unscheduled. Must not contain pods with scheduling gates.
	Pods []PodInfo `json:"pods,omitempty"`
	// Nodes are all nodes in the cluster.
	Nodes []NodeInfo `json:"nodes,omitempty"`
	// PVs are the bound PersistentVolumes in the cluster (i.e. those with a populated claimRef).
	PVs []PVInfo `json:"pvs,omitempty"`
	// PVCs are the PersistentVolumeClaims in the cluster.
	PVCs []PVCInfo `json:"pvcs,omitempty"`
	// StorageClasses are the storage classes present in the cluster.
	StorageClasses []storagev1.StorageClass `json:"storageClasses,omitempty"`
	// PriorityClasses are the priority classes present in the cluster.
	PriorityClasses []schedulingv1.PriorityClass `json:"priorityClasses,omitempty"`
	// RuntimeClasses are the runtime classes present in the cluster.
	RuntimeClasses []nodev1.RuntimeClass `json:"runtimeClasses,omitempty"`
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
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// Overhead is the resource overhead for the Pod's sandbox, as defined by the pod's RuntimeClass.
	Overhead corev1.ResourceList `json:"overhead,omitempty"`
	// NodeName is the name of the node where the Pod is scheduled.
	NodeName string `json:"nodeName,omitempty"`
	// SchedulerName is the name of the scheduler that should be used to schedule the Pod.
	SchedulerName string `json:"schedulerName,omitempty"`
	// PriorityClassName is the name of the priority class that should be used to schedule the Pod.
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// PreemptionPolicy is the policy for preempting pods with lower priority.
	PreemptionPolicy corev1.PreemptionPolicy `json:"preemptionPolicy,omitempty"`
	// RuntimeClassName is the name of the RuntimeClass used to run the Pod.
	RuntimeClassName  string `json:"runtimeClassName,omitempty"`
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
type GetNodeScorer func(scoringStrategy apicommon.NodeScoringStrategy) (NodeScorer, error)

// NodeScorer defines an interface for computing node scores for scaling decisions.
type NodeScorer interface {
	// Score computes the node score given the NodeScorerArgs. On failure, it must return an error with the sentinel error api.ErrComputeNodeScore
	Score(ctx context.Context, args NodeScorerArgs) (NodeScore, error)
	// Select selects the winning NodeScore amongst the NodeScores of a given simulation pass and returns the pointer to the same.
	// If there is no winning node score amongst the group, then it returns nil.
	Select(ctx context.Context, groupNodeScores []NodeScore) (winningNodeScore *NodeScore, err error)
}

// NodeScorerArgs contains arguments for node scoring computation.
type NodeScorerArgs struct {
	// ID that must be given to the NodeScore produced by the NodeScorer.
	ID string `json:"id"`
	// ResourceWeights maps resource names to their relative weights for scoring.
	// Propagated from [Request.ResourceWeights]; if nil the scorer uses its built-in defaults.
	ResourceWeights map[corev1.ResourceName]float64 `json:"resourceWeights,omitempty"`
	// InstancePrices lists pricing data for all instance types in the request.
	// Propagated from [Request.InstancePrices]; used by cost-aware scoring strategies.
	InstancePrices []sacorev1alpha1.InstancePrice `json:"instancePrices,omitempty"`
	// ScaledNodePlacement represents the placement information for the Node.
	ScaledNodePlacement sacorev1alpha1.NodePlacement `json:"scaledNodePlacement"`
	// ScaledNodePodAssignment represents the node-pod assignment of the scaled Node for the current run.
	ScaledNodePodAssignment *simulationapi.NodePodAssignment `json:"scaledNodePodAssignment,omitempty"`
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of the ClusterSnapshot,
	// or it is a winning simulated Node from a previous run.
	OtherNodePodAssignments []simulationapi.NodePodAssignment `json:"otherNodePodAssignments,omitempty"`
	// LeftOverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after simulation is completed.
	LeftOverUnscheduledPods []apicommon.NamespacedName `json:"leftOverUnscheduledPods,omitempty"`
}

// NodeScore represents the scoring result for a node in scaling simulations.
type NodeScore struct {
	// ResourceWeights carries per-resource weights from the request for use in weight-aware Select tie-breaking.
	ResourceWeights map[corev1.ResourceName]float64 `json:"resourceWeights,omitempty"`
	// Placement represents the placement information for the Node.
	Placement          sacorev1alpha1.NodePlacement   `json:"placement"`
	ScaledNodeResource simulationapi.NodeResourceInfo `json:"scaledNodeResource"`
	// Name uniquely identifies this NodeScore
	Name            string                     `json:"name"`
	UnscheduledPods []apicommon.NamespacedName `json:"unscheduledPods,omitempty"`
	// InstancePrices carries the pricing data from the request for use in cost-aware Select tie-breaking.
	InstancePrices []sacorev1alpha1.InstancePrice `json:"instancePrices,omitempty"`
	// Value is the score value for this Node.
	Value int `json:"value"`
}

// SimulatorConfig holds the configuration for the internal simulator used by the scaling advisor planner.
type SimulatorConfig struct {
	// UtilizationThresholds is the resource utilization thresholds for a node to be considered underutilized and thus
	// a candidate for scale-in.
	// The keys are the resource names such as `cpu`/`memory`/`gpu`/etc. and the values are the corresponding
	// utilization thresholds expressed as fractions from 0 to 1.
	UtilizationThresholds map[corev1.ResourceName]float64 `json:"utilizationThresholds,omitempty"`
	simulationapi.SimulationConfig
	// UnderutilizedDuration is the duration for which a node should be under the utilization thresholds to be
	// considered a candidate for scale-in.
	UnderutilizedDuration time.Duration `json:"underutilizedDuration,omitzero"`
	// MaxParallelSimulations is the maximum number of parallel simulations that can be run by the scaling advisor planner.
	MaxParallelSimulations int `json:"maxParallelSimulations,omitzero"`
}

// ScalingPlannerArgs encapsulates the arguments required to create a ScalingPlanner.
type ScalingPlannerArgs struct {
	// ViewAccess provides access to the MinKAPI views.
	ViewAccess minkapi.ViewAccess
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
	GetScaleInSimulator(args SimulatorArgs) (ScaleInSimulator, error)
}

// SimulatorArgs is an encapsulation of the arguments used to create a ScaleOutSimulator or ScaleInSimulator.
// Not all the fields may be necessary for constructing a specific simulator implementation.
type SimulatorArgs struct {
	// ViewAccess holds the minkapi ViewAccess used to create views against which simulations are run.
	ViewAccess minkapi.ViewAccess
	// SchedulerLauncher holds the launcher for the embedded kube-scheduler
	SchedulerLauncher simulationapi.SchedulerLauncher
	// NodeScorer holds the facade to compute NodeScores for simulated scaled nodes.
	NodeScorer NodeScorer
	// Request is the planner request to be satisfied by this simulator.
	Request *Request
	// Strategy holds the simulator strategy which customizes simulator implementation and behavior
	Strategy apicommon.SimulatorStrategy
	// TraceDir is the base directory for storing trace logs and other dump data by the simulator
	TraceDir string
	// Config holds the static simulator config parameters
	Config SimulatorConfig
}

// ScaleOutSimulator is a facade that executes [simulationapi.Simulation]'s to generate one or more [ScaleOutPlanResult]'s sent
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
// Will run the different scale-out [simulation.Simulation]'s of a [simulation.SimulationGroup] concurrently where each
// simulation Run virtually
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

	// Simulate is the high level activity that runs [Simulation]'s created from given [SimulationFactory] with the given
	// planner [Request] against the given simulationView and delivers [ScaleOutPlanResult]'s on the planResult channel.
	Simulate(ctx context.Context, simulationFactory simulationapi.SimulationFactory, simulationView minkapi.View) <-chan ScaleOutPlanResult
}

// ScaleInSimulator is a facade that executes [simulationapi.Simulation]'s to generate one or more [ScaleInPlanResult]'s sent
// on a result channel.
type ScaleInSimulator interface {
	io.Closer
	// Simulate is the high level activity that runs [Simulation]'s created from given [SimulationFactory] with the given
	// planner [Request] against the given simulationView and delivers [ScaleInPlanResult]'s on the planResult channel.
	Simulate(ctx context.Context, simulationFactory simulationapi.SimulationFactory, simulationView minkapi.View) <-chan ScaleInPlanResult
}

// ScaleInCandidateSelector is the facade for selection of all scale-in candidate nodes while honoring constraints.
type ScaleInCandidateSelector interface {
	// Select returns all scale-in candidate nodes in [ScaleInCandidateSelectorArgs.View] as a slice of [corev1.Node]
	// while honoring [ScaleInCandidateSelectorArgs.UtilizationThresholds]'s and [PodDisruptionBudgets].
	//
	// [PodDisruptionBudgets]: https://kubernetes.io/docs/reference/kubernetes-api/policy/pod-disruption-budget-v1/
	Select(ctx context.Context, args ScaleInCandidateSelectorArgs) ([]corev1.Node, error)
}

// ScaleInCandidateSelectorArgs encapsulates the input for the [ScaleInCandidateSelector].
type ScaleInCandidateSelectorArgs struct {
	// View is the minkapi cluster view
	View minkapi.View
	// UtilizationThresholds is the resource utilization thresholds below which a node is considered underutilized and thus
	// a candidate for scale-in. The keys are the resource names such as `cpu`/`memory`/`gpu`/etc. and the values are the
	// corresponding utilization thresholds expressed as fractions from 0 to 1.
	UtilizationThresholds map[corev1.ResourceName]float64
	// PoolMinLimits carries the NodePool min resource limits by pool name. Nodes should not be scaled-in if it would violate the
	// pool minimum limits.
	// (This should be taken from [sacorev1alpha1.ResourceLimits.Min] taken from [sacorev1alpha1.NodeTemplateRef.Quota]
	// within [sacorev1alpha1.NodePool.NodeTemplateRefs]).
	PoolMinLimits map[string]corev1.ResourceList
	// PoolPriorities carries the pool priority by pool name. Lower priority valued nodes should be preferenced for scale-in
	// over higher-priority valued nodes.
	PoolPriorities map[string]int32
}

// ScaleOutPlanResult represents a result from the ScaleOutSimulator.Simulate
type ScaleOutPlanResult struct {
	Labels       map[string]string            `json:"labels,omitempty"`
	ScaleOutPlan *sacorev1alpha1.ScaleOutPlan `json:"scaleOutPlan,omitempty"`
	// Error is any error encountered during plan generation. Represents a terminal error; no further results will be sent.
	Error error `json:"error,omitempty"`
}

// ScaleInPlanResult is one delivery from [ScaleInSimulator.Simulate].
type ScaleInPlanResult struct {
	Memento     ScaleInMemento              `json:"scaleInMemento,omitempty"`
	View        minkapi.View                `json:"-"`
	Labels      map[string]string           `json:"labels,omitempty"`
	ScaleInPlan *sacorev1alpha1.ScaleInPlan `json:"scaleInPlan,omitempty"`
	// Error, when non-nil, is terminal; the channel is closed after delivering it.
	Error error `json:"error,omitempty"`
}

// ScaleOutSimGroupPassScores represents the scoring results, including the winner score, for a single pass of a ScaleOutSimGroup
// after running the NodeScorer against the ScaleOutSimResult's of the pass.
type ScaleOutSimGroupPassScores struct {
	// WinnerScore is the highest scoring node in the group.
	WinnerScore *NodeScore `json:"winnerScore,omitempty"`
	// WinnerNode is the actual node corresponding to the winner score.
	WinnerNode *corev1.Node `json:"winnerNode,omitempty"`
	// AllScores contains all computed node scores for the group.
	AllScores []NodeScore `json:"allScores,omitempty"`
}

// ScaleOutSimGroupCycleResult represents the result of running all passes for a ScaleOutSimGroup.
type ScaleOutSimGroupCycleResult struct {
	// NextGroupPassView is the updated view after executing all passes in this group.
	// The next group, if any, should use this view as its base view for its overlay view.
	NextGroupPassView minkapi.View `json:"-"`
	// Name is the name of the simulation group.
	Name string `json:"name"`
	// WinnerNodeScores contains the node scores of the winning nodes.
	WinnerNodeScores []NodeScore `json:"winnerNodeScores,omitempty"`
	// LeftoverUnscheduledPods contains the namespaced names of pods that could not be scheduled.
	LeftoverUnscheduledPods []apicommon.NamespacedName `json:"leftoverUnscheduledPods,omitempty"`
	// PassNum is the number of passes executed in this group before moving to the next group.
	// A pass is defined as the execution of all simulations in a group.
	PassNum int `json:"passNum"`
}

// Memento is an encapsulation of partial details of a [sacorev1alpha1.ScaleOutPlan] and/or [sacorev1alpha1.ScaleInPlan]
// that can be used by subsequent invocations of [ScalingPlanner] to optimize execution.
type Memento struct {
	// ScaleIn holds the [ScaleInMemento] used by the next invocation of the [ScaleInSimulator].
	ScaleIn ScaleInMemento `json:"scaleIn"`
	// ScaleOut holds the [ScaleOutMemento] used by the next invocation of the [ScaleOutSimulator].
	ScaleOut ScaleOutMemento `json:"scaleOut"`
}

// ScaleInMemento carries planner cross-invocation timestamps so the [ScaleInSimulator] requires under-utilization to be sustained
// before recommending a node for removal in the [ScaleInPlanResult].
type ScaleInMemento struct {
	// LastIdentifiedUnneededNodes maps node name to the time it was first observed as a
	// successful scale-in candidate. The node name will be present in [sacorev1alpha1.ScaleInItem] inside [ScaleInPlanResult] produced
	// by the [ScaleInSimulator] only after it has stayed in this map for the configured  [SimulatorConfig.UnderutilizedDuration]
	LastIdentifiedUnneededNodes map[string]time.Time `json:"lastIdentifiedUnneededNodes,omitempty"`
	// LastUnderutilizedSinceNodes maps node name to the last time it was observed below the
	// utilization threshold. Only nodes that have been underutilized beyond [SimulatorConfig.UnderutilizedDuration]
	// are considered as candidates for scale-in by the [ScaleInSimulator]
	LastUnderutilizedSinceNodes map[string]time.Time `json:"lastUnderutilizedSinceNodes,omitempty"`
}

// ScaleOutMemento encapsulates partial details from previous [sacorev1alpha1.ScaleOutPlan] that can be used by the
// [ScalingPlanner] in its next invocation.
type ScaleOutMemento struct {
	// UpcomingNodePlacements contains [sacorev1alpha1.NodePlacement]'s that were provided in previously accepted
	// [sacorev1alpha1.ScaleOutItem]'s plans but do not have `Ready` nodes yet or have `Ready` nodes, but are still tainted
	// with any of [sacorev1alpha1.NodePool]'s `startupTaintKeys`. The [ScalingPlanner] should consider these nodes in its planning.
	UpcomingNodePlacements map[sacorev1alpha1.NodePlacement]int `json:"upcomingNodePlacements"`
}

// ScalingPlannerService is the facade for the scaling planner microservice that embeds a ScalingPlanner
// Offers a REST API for the embedded ScalingPlanner
type ScalingPlannerService interface {
	apicomponent.Service
	ScalingPlanner
}

// ScalingPlannerServiceConfig holds the service configuration for the scaling planner microservice.
type ScalingPlannerServiceConfig struct {
	// CloudProvider is the cloud provider for which the scaling advisor planner is initialized.
	CloudProvider apicommon.CloudProvider `json:"cloudProvider"`
	// TraceDir is the base directory for storing trace files produced by the scaling advisor planner.
	TraceDir string `json:"traceDir,omitempty"`
	// SimulatorConfig holds the configuration used by the internal simulator.
	SimulatorConfig SimulatorConfig `json:"simulatorConfig"`
	// ServerConfig holds the server configuration for the scaling advisor planner.
	ServerConfig apicomponent.ServerConfig `json:"serverConfig"`
	// MinKAPIConfig holds the configuration for the MinKAPI server used by the scaling advisor planner.
	MinKAPIConfig minkapi.Config `json:"minkAPIConfig"`
	// ClientConfig holds the client QPS and Burst settings for the scaling advisor planner.
	ClientConfig apicomponent.QPSBurst `json:"clientConfig"`
}

// Factories is a struct that holds all planner factories.
type Factories struct {
	Planner    ScalingPlannerFactory
	Simulator  SimulatorFactory
	Simulation simulationapi.SimulationFactory
}
