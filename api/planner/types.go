// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"fmt"
	"io"
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
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	SimulatorStrategy commontypes.SimulatorStrategy `json:"simulatorStrategy,omitempty"`
	// ScoringStrategy defines the node scoring strategy to use for scaling decisions.
	ScoringStrategy commontypes.NodeScoringStrategy `json:"scoringStrategy,omitempty"`
	// AdviceGenerationMode defines the mode in which scaling advice is generated.
	AdviceGenerationMode commontypes.ScalingAdviceGenerationMode `json:"adviceGenerationMode,omitempty"`
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
func (p *PodInfo) GetResourceInfo() PodResourceInfo {
	return PodResourceInfo{
		NamespacedName: commontypes.NamespacedName{
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
		PoolName:         n.Labels[commonconstants.LabelNodePoolName],
		TemplateName:     n.Labels[commonconstants.LabelNodeTemplateName],
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
	ClaimRef          commontypes.NamespacedName   `json:"claimRef,omitzero"`
	StorageClassName  string                       `json:"storageClassName,omitempty"`
	Phase             corev1.PersistentVolumePhase `json:"phase,omitempty"`
	metav1.ObjectMeta `json:",inline"`
	VolumeMode        corev1.PersistentVolumeMode         `json:"volumeMode,omitempty"`
	AccessModes       []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
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
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of the ClusterSnapshot,
	// or it is a winning simulated Node from a previous run.
	OtherNodePodAssignments []NodePodAssignment
	// LeftOverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after simulation is completed.
	LeftOverUnscheduledPods []commontypes.NamespacedName
}

// NodeScore represents the scoring result for a node in scaling simulations.
type NodeScore struct {
	ScaledNodeResource NodeResourceInfo
	// Placement represents the placement information for the Node.
	Placement sacorev1alpha1.NodePlacement
	// Name uniquely identifies this NodeScore
	Name            string
	UnscheduledPods []commontypes.NamespacedName
	// Value is the score value for this Node.
	Value int
}

// NodePodAssignment represents the assignment of pods to a node for simulation purposes.
type NodePodAssignment struct {
	// NodeResources contains the resource information for the node.
	NodeResources NodeResourceInfo
	// ScheduledPods contains the list of pods scheduled to this node.
	ScheduledPods []PodResourceInfo
}

// VolumeClaimAssignment represents the assignment of a PersistentVolumeClaim to a PersistentVolume
type VolumeClaimAssignment struct {
	// ClaimName is the PVC namespaced name.
	ClaimName commontypes.NamespacedName
	// VolumeName is the name of the bound PV
	VolumeName string
}

// ResourceWeigher defines an interface for obtaining resource weights for scoring.
type ResourceWeigher interface {
	// GetWeights returns the resource weights for the given instance type.
	GetWeights(instanceType string) (map[corev1.ResourceName]float64, error)
}

// StorageMetaAccess defines an interface for querying misc storage metadata
type StorageMetaAccess interface {
	// GetFallbackCSINodeSpec gets the default storagev1.CSINodeSpec which is suitable for the given instanceType.
	// Used as a fallback when there is no CSINodeSpec associated with the NodeInfo or in a scale-from-zero
	// scenario.
	GetFallbackCSINodeSpec(instanceType string) (storagev1.CSINodeSpec, error)
}

// PodResourceInfo contains resource information for a pod used in scoring calculations.
type PodResourceInfo struct {
	// AggregatedRequests is an aggregated resource requests for all containers of the Pod.
	AggregatedRequests         corev1.ResourceList `json:"aggregatedRequests,omitempty"`
	commontypes.NamespacedName `json:",inline"`
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

// SimulatorConfig holds the configuration for the internal simulator used by the scaling advisor planner.
type SimulatorConfig struct {
	// MaxParallelSimulations is the maximum number of parallel simulations that can be run by the scaling advisor planner.
	MaxParallelSimulations int
	// TrackPollInterval is the polling interval for tracking pod scheduling in the view of the simulator.
	TrackPollInterval time.Duration
	// MaxUnchangedTrackAttempts is the maximum number of unchanged simulation track attempts after which a simulation run is
	// considered as stabilized.
	MaxUnchangedTrackAttempts int
	// BindVolumeClaimsForImmediateMode should be set if simulator is expected to bind unbound PVC<->PV for
	// [corev1.VolumeBindingImmediate], also creating a simulated PV if a matching existing PV doesn't exist.
	BindVolumeClaimsForImmediateMode bool
}

// ScalingPlannerArgs encapsulates the arguments required to create a ScalingPlanner.
type ScalingPlannerArgs struct {
	// ViewAccess provides access to the MinKAPI views.
	ViewAccess minkapi.ViewAccess
	// ResourceWeigher provides resource weights for scoring.
	ResourceWeigher ResourceWeigher
	// PricingAccess provides access to instance pricing information.
	PricingAccess pricing.InstancePricingAccess
	// StorageMetaAccess provides access to storage metadata.
	StorageMetaAccess StorageMetaAccess
	// SchedulerLauncher provides functionality to launch kube-scheduler instances.
	SchedulerLauncher SchedulerLauncher
	// SimulatorFactory is the factory facade to create simulators
	SimulatorFactory SimulatorFactory
	// SimulationFactory is the factory facade to create simulations.
	SimulationFactory SimulationFactory
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

// SimulationFactory is a factory facade for creating Simulation objects
type SimulationFactory interface {
	// NewScaleOut creates a ScaleOutSimulation instance with the given name and arguments.
	NewScaleOut(args ScaleOutSimArgs) (ScaleOutSimulation, error)
	// TODO: Add NewScaleIn method here.
}

// SimulatorArgs is an encapsulation of the arguments used to create a ScaleOutSimulator or ScaleInSimulator.
// Not all the fields may be necessary for constructing a specific simulator implementation.
type SimulatorArgs struct {
	// ViewAccess holds the minkapi ViewAccess used to create views against which simulations are run.
	ViewAccess minkapi.ViewAccess
	// SchedulerLauncher holds the launched for the embedded kube-scheduler
	SchedulerLauncher SchedulerLauncher
	// StorageMetaAccess holds the access facade to storage metadata.
	StorageMetaAccess StorageMetaAccess
	// NodeScorer holds the facade to compute NodeScores for simulated scaled nodes.
	NodeScorer NodeScorer
	// Strategy holds the simulator strategy which customizes simulator implementation and behaviorchanges simulator implementation and behavior
	Strategy commontypes.SimulatorStrategy
	// TraceDir is the base directory for storing trace logs and other dump data by the simulator
	TraceDir string
	// Config holds the static simulator config parameters
	Config SimulatorConfig
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
	// TraceDir is the base directory for storing trace files produced by the scaling advisor planner.
	TraceDir string
	// ServerConfig holds the server configuration for the scaling advisor planner.
	ServerConfig commontypes.ServerConfig
	// MinKAPIConfig holds the configuration for the MinKAPI server used by the scaling advisor planner.
	MinKAPIConfig minkapi.Config
	// ClientConfig holds the client QPS and Burst settings for the scaling advisor planner.
	ClientConfig commontypes.QPSBurst
	// SimulatorConfig holds the configuration used by the internal simulator.
	SimulatorConfig SimulatorConfig
}

// Factories is a struct that holds all planner factories.
type Factories struct {
	Planner         ScalingPlannerFactory
	Simulator       SimulatorFactory
	Simulation      SimulationFactory
	ResourceWeigher ResourceWeigher
}
