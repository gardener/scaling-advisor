// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// NamespacedName is a fully qualified object name.
// NOTE: This is only needed since k8s APIMachinery types.NamespacedName does not have JSON tags and k8s maintainers
// recommended that every project should use their own copy of NamespacedName.
type NamespacedName struct {
	// Namespace is the namespace of the object.
	Namespace string `json:"namespace,omitempty"`
	// Name is the name of the object.
	Name string `json:"name"`
}

// AsObjectName converts this namespaced name to a client-go cache.ObjectName
func (nn NamespacedName) AsObjectName() cache.ObjectName {
	return cache.ObjectName{Name: nn.Name, Namespace: nn.Namespace}
}

// AsObjectReference constructs an ObjectReference referring this name or nil if name is empty.
func (nn NamespacedName) AsObjectReference() *corev1.ObjectReference {
	if nn.Name == "" {
		return nil
	} else {
		return &corev1.ObjectReference{Namespace: nn.Namespace, Name: nn.Name}
	}
}

// String returns the general purpose string representation.
// Matches implementation in APIMachinery types.NamespacedName
func (nn NamespacedName) String() string {
	return nn.Namespace + string(types.Separator) + nn.Name
}

// PlannerStrategy represents a scaling planner's strategy that governs the approach it takes towards generating scaling plans.
// +kubebuilder:validation:Enum=scale-in-then-scale-out;scale-out-then-scale-in;consolidated-scale-out;only-scale-out;only-scale-in
type PlannerStrategy string

const (
	// PlannerStrategyScaleInThenScaleOut represents a planner strategy which executes pure scale-in simulation(s) first
	// to produce scale-in plans and then executes pure scale-out simulation(s) according to [SimulatorStrategy] to
	// produce scale-out plans.
	// Pure scale-in simulations involve scaling-in underutilized nodes and checking that existing scheduled workload
	// can be successfully moved to existing nodes. Pure scale-out simulations involve scaling out nodes and checking that
	// unscheduled workload can be scheduled on them.
	// The simulation view is passed from last successful scale-in simulation to the scale-out simulation.
	PlannerStrategyScaleInThenScaleOut PlannerStrategy = "scale-in-then-scale-out"

	// PlannerStrategyScaleOutThenScaleIn represents a planner strategy which executes pure scale-out simulation(s) first
	// to produce scale-out plans and then executes pure scale-in simulation(s) to produce scale-in plans.
	// The simulation view is passed from last successful scale-out simulation to the first scale-in simulation.
	PlannerStrategyScaleOutThenScaleIn PlannerStrategy = "scale-out-then-scale-in"

	// PlannerStrategyConsolidatedScaleOut represents a planner strategy in which existing scheduled workload on candidate scale-in
	//  node(s) are unassigned and added to the unscheduled workload, the nodes then removed (without any simulation), followed
	// by scale-out simulation(s) using the configured [SimulatorStrategy]. Then a consolidated scale-in and scale-out plan
	// is produced.
	PlannerStrategyConsolidatedScaleOut PlannerStrategy = "consolidated-scale-out"

	// PlannerStrategyOnlyScaleOut represents a planner strategy in which only scale-out plans are produced via applicable
	// scale-out simulation(s).
	PlannerStrategyOnlyScaleOut PlannerStrategy = "only-scale-out"

	// PlannerStrategyOnlyScaleIn represents a planner strategy in which only scale-in plans are produced via applicable
	// scale-in simulation(s).
	PlannerStrategyOnlyScaleIn PlannerStrategy = "only-scale-in"
)

// SupportedPlannerStrategies is a set of all supported planner strategies.
var SupportedPlannerStrategies = sets.New(
	PlannerStrategyScaleInThenScaleOut,
	PlannerStrategyScaleOutThenScaleIn,
	PlannerStrategyConsolidatedScaleOut,
	PlannerStrategyOnlyScaleOut,
	PlannerStrategyOnlyScaleIn,
)

// SimulatorStrategy represents a simulator strategy variant.
// +kubebuilder:validation:Enum=single-node-multi-sim;multi-node-single-sim
type SimulatorStrategy string

const (
	// SimulatorStrategySingleNodeMultiSim represents a simulator strategy which runs independent multiple simulations differentiated by scaling a single node for a combination
	// of NodePool, NodeTemplate and AvailabilityZone.
	SimulatorStrategySingleNodeMultiSim SimulatorStrategy = "single-node-multi-sim"
	// SimulatorStrategyMultiNodeSingleSim represents a simulator strategy which runs a single simulation by scaling multiple nodes for
	// all combinations of NodePool, NodeTemplate and AvailabilityZone.
	SimulatorStrategyMultiNodeSingleSim SimulatorStrategy = "multi-node-single-sim"
)

// IsMultiNode returns true if the strategy scales multiple nodes, false otherwise.
func (s SimulatorStrategy) IsMultiNode() bool {
	return s == SimulatorStrategyMultiNodeSingleSim
}

// IsSingleNode returns true if the strategy scales only a single node, false otherwise.
func (s SimulatorStrategy) IsSingleNode() bool {
	return s == SimulatorStrategySingleNodeMultiSim
}

// SupportedSimulatorStrategies is a set of all supported simulator strategies.
var SupportedSimulatorStrategies = sets.New(
	SimulatorStrategySingleNodeMultiSim,
	SimulatorStrategyMultiNodeSingleSim,
)

// ScalingAdviceGenerationMode defines the mode in which scaling advice is generated.
// +kubebuilder:validation:Enum=incremental;all-at-once
// +enum
type ScalingAdviceGenerationMode string

const (
	// ScalingAdviceGenerationModeIncremental is the mode in which scaling advice is generated incrementally.
	// In this mode, scaling advisor will dish out scaling advice as soon as it has the first scale-out/in advice from a simulation run.
	ScalingAdviceGenerationModeIncremental ScalingAdviceGenerationMode = "incremental"
	// ScalingAdviceGenerationModeAllAtOnce is the mode in which scaling advice is generated all at once.
	// In this mode, scaling advisor will generate scaling advice after it has run the complete set of simulations wher either
	// all pending pods have been scheduled or stabilised.
	ScalingAdviceGenerationModeAllAtOnce ScalingAdviceGenerationMode = "all-at-once"
)

// SupportedAdviceGenerationModes is a set of all supported scaling advice generation modes.
var SupportedAdviceGenerationModes = sets.New(
	ScalingAdviceGenerationModeIncremental,
	ScalingAdviceGenerationModeAllAtOnce,
)

// IsIncremental returns true if the advice generation mode is incremental.
func (a ScalingAdviceGenerationMode) IsIncremental() bool {
	return a == ScalingAdviceGenerationModeIncremental
}

// IsAllAtOnce returns true if the advice generation mode is all-at-once.
func (a ScalingAdviceGenerationMode) IsAllAtOnce() bool {
	return a == ScalingAdviceGenerationModeAllAtOnce
}

// NodeScoringStrategy represents a node scoring strategy variant.
// +kubebuilder:validation:Enum=least-waste;least-cost
type NodeScoringStrategy string

const (
	// NodeScoringStrategyLeastWaste represents a scoring strategy that minimizes resource waste.
	NodeScoringStrategyLeastWaste NodeScoringStrategy = "least-waste"
	// NodeScoringStrategyLeastCost represents a scoring strategy that minimizes cost.
	NodeScoringStrategyLeastCost NodeScoringStrategy = "least-cost"
)

// SupportedNodeScoringStrategies is a set of all supported node scoring strategies.
var SupportedNodeScoringStrategies = sets.New(
	NodeScoringStrategyLeastWaste,
	NodeScoringStrategyLeastCost,
)

// CloudProvider represents the cloud provider type for the cluster.
// +kubebuilder:validation:Enum=aws;gcp;azure;ali;openstack
type CloudProvider string

const (
	// CloudProviderAWS indicates AWS as cloud provider.
	CloudProviderAWS CloudProvider = "aws"
	// CloudProviderGCP indicates GCP as cloud provider.
	CloudProviderGCP CloudProvider = "gcp"
	// CloudProviderAzure indicates Azure as cloud provider.
	CloudProviderAzure CloudProvider = "azure"
	// CloudProviderAli indicates Alibaba Cloud as cloud provider.
	CloudProviderAli CloudProvider = "ali"
	// CloudProviderOpenStack indicates OpenStack as cloud provider.
	CloudProviderOpenStack CloudProvider = "openstack"
)

// AsCloudProvider converts a string to CloudProvider type. It returns an error if the cloudProvider string
// is not supported.
func AsCloudProvider(cloudProvider string) (CloudProvider, error) {
	switch cloudProvider {
	case "aws":
		return CloudProviderAWS, nil
	case "gcp":
		return CloudProviderGCP, nil
	case "azure":
		return CloudProviderAzure, nil
	case "ali":
		return CloudProviderAli, nil
	case "openstack":
		return CloudProviderOpenStack, nil
	default:
		return "", fmt.Errorf("%w: unknown %q", ErrUnsupportedCloudProvider, cloudProvider)
	}
}

// ContextKey is the type alias for scaling advisor related context keys
type ContextKey string

const (
	// VerbosityCtxKey is the context key indicating the diagnostic/log verbosity.
	VerbosityCtxKey ContextKey = "verbosity"

	// TraceDirCtxKey is the context key under which the dir path to the trace dir is stored.
	TraceDirCtxKey ContextKey = "trace-dir"

	// TraceLogPathCtxKey is the context key under which the path to the trace log file is stored.
	TraceLogPathCtxKey ContextKey = "trace-log"
)

// CapacityType identifies the procurement model of compute capacity used to run a node, such as on-demand or spot.
// +kubebuilder:validation:Enum=on-demand;spot
type CapacityType string

const (
	// CapacityTypeOnDemand represents on-demand compute capacity.
	CapacityTypeOnDemand CapacityType = "on-demand"
	// CapacityTypeSpot represents spot or preemptible compute capacity.
	CapacityTypeSpot CapacityType = "spot"
)
