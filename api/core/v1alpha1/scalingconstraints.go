// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"slices"

	apicommon "github.com/gardener/scaling-advisor/api/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={scon},scope=Namespaced
// +kubebuilder:printcolumn:name="PlannerStrategy",type=string,JSONPath=`.spec.plannerStrategy`
// +kubebuilder:printcolumn:name="SimulatorStrategy",type=string,JSONPath=`.spec.simulatorStrategy`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ScalingConstraint defines the inputs and constraints used to generate [ScalingAdvice] for a cluster.
type ScalingConstraint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec specifies the inputs and constraints used to generate [ScalingAdvice].
	// +required
	Spec ScalingConstraintSpec `json:"spec"`
	// Status contains validation and processing information for this ScalingConstraint.
	// +optional
	Status ScalingConstraintStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingConstraintList is a list of ScalingConstraint.
type ScalingConstraintList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ScalingConstraint's.
	// +required
	Items []ScalingConstraint `json:"items"`
}

// ScalingConstraintSpec defines the constraints used to generate [ScalingAdvice].
type ScalingConstraintSpec struct {
	// SimulatorStrategy defines the simulator strategy used by the scaling planner.
	// +optional
	SimulatorStrategy apicommon.SimulatorStrategy `json:"simulatorStrategy,omitempty"`
	// ScoringStrategy selects the algorithm used to score and rank candidate scale-out nodes.
	// Only relevant when [SimulatorStrategy] is [apicommon.SimulatorStrategySingleNodeMultiSim].
	// +optional
	ScoringStrategy apicommon.NodeScoringStrategy `json:"scoringStrategy,omitempty"`
	// PlannerStrategy defines the high-level approach the scaling planner uses to combine scale-in and scale-out
	// simulation results into a final [ScalingAdvice]. It controls the order and scope of simulations performed.
	// +required
	PlannerStrategy apicommon.PlannerStrategy `json:"plannerStrategy"`
	// NodePools is the list of node pools to choose from when creating scaling advice.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	// +required
	NodePools []NodePool `json:"nodePools"`
}

// GetAllAvailabilityZones gets all the availability zones across all node pools as a sorted slice.
func (c *ScalingConstraintSpec) GetAllAvailabilityZones() []string {
	zoneSet := sets.New[string]()
	for _, p := range c.NodePools {
		zoneSet.Insert(p.AvailabilityZones...)
	}
	zones := zoneSet.UnsortedList()
	slices.Sort(zones)
	return zones
}

// ConditionTypeAccepted is the condition type set on a [ScalingConstraint] by the reconciler
// to indicate whether the constraint has been successfully validated and accepted as input.
// Follows the convention established by Gateway API (GatewayConditionAccepted, RouteConditionAccepted).
const ConditionTypeAccepted = "Accepted"

// ScalingConstraintStatus reports validation and processing status for a [ScalingConstraint].
type ScalingConstraintStatus struct {
	// Conditions contains the conditions for the ScalingConstraint.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodePool defines a scalable node pool together with the [NodeTemplate]s that may be used to provision nodes in the
// pool.
//
//nolint:govet // fieldalignment: intentional layout for k8s CRD / wire compatibility
type NodePool struct {
	Labels           map[string]string `json:"labels,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	ScalingParams    *ScalingParams    `json:"scalingParams,omitempty"`
	Name             string            `json:"name"`
	Region           string            `json:"region"`
	Taints           []corev1.Taint    `json:"taints,omitempty"`
	StartupTaintKeys []string          `json:"startupTaintKeys,omitempty"`
	// AvailabilityZones is the list of availability zones in which nodes may be provisioned for this pool.
	// +kubebuilder:validation:MinItems=1
	AvailabilityZones []string `json:"availabilityZones"`
	// NodeTemplateRefs is the list of node templates that may be used to provision nodes in this pool.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	NodeTemplateRefs []NodeTemplateRef `json:"nodeTemplateRefs"`
	// Priority defines the preference for this NodePool when selecting a scale-out candidate or scale-in candidate.
	//
	// If omitted, the default implicit priority is zero. Higher values are preferred for scale-out. Lower values are
	// preferred for scale-in.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Priority int32 `json:"priority,omitempty"`
}

// NodeTemplateRef references a [NodeTemplate] and specifies how it may be used within its parent [NodePool], including
// its relative priority and optional [ResourceLimits].
type NodeTemplateRef struct {
	// Quota specifies optional resource limits for the referenced NodeTemplate within the parent NodePool.
	// +optional
	Quota *ResourceLimits `json:"quota,omitempty"`
	// Name is the name of the referenced [NodeTemplate]. The referenced [NodeTemplate] must be defined in a [NodeTemplateSet].
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
	// Priority defines the preference for this NodeTemplate when selecting a scale-out candidate in the parent NodePool.
	//
	// If omitted, the default implicit priority is zero. Higher values are preferred.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Priority int32 `json:"priority,omitempty"`
}

// ResourceLimits specify the minimum and maximum aggregate resources that may be provisioned for the referenced
// [NodeTemplate] within a [NodePool].
type ResourceLimits struct {
	// Min specifies the minimum aggregate resources that may be provisioned.
	// +optional
	Min corev1.ResourceList `json:"min,omitempty"`
	// Max specifies the maximum aggregate resources that may be provisioned.
	// +optional
	Max corev1.ResourceList `json:"max,omitempty"`
}

// ScalingParams specifies the scaling parameters to be used when scaling a node pool.
//
//nolint:govet // fieldalignment: intentional layout for k8s CRD / wire compatibility
type ScalingParams struct {
	UtilizationThresholds []ResourceUtilizationThreshold `json:"utilizationThresholds,omitempty"`
	UnderutilizedDuration metav1.Duration                `json:"underutilizedDuration,omitempty"`
}

// ResourceUtilizationThreshold pairs a resource name with its utilization threshold.
type ResourceUtilizationThreshold struct {
	// Name is the resource name (e.g. "cpu", "memory", "nvidia.com/gpu").
	// +required
	Name corev1.ResourceName `json:"name"`
	// Threshold is the utilization fraction at or below which the node is considered underutilized,
	// expressed as a decimal string in the range (0, 1].
	// Examples: "0.5" (50 %), "1" (100 %).
	//
	// Using string rather than float64 follows the same convention as [ResourceUnitPrice.Value] and
	// other Kubernetes ecosystem projects to avoid floating-point serialization issues in CRD fields.
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	// +required
	Threshold string `json:"threshold"`
}
