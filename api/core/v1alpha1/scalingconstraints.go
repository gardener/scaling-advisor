// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
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
// +kubebuilder:resource:shortName={sc}

// ScalingConstraint defines the constraints used by the scaling advisor to generate scaling advice for a cluster.
type ScalingConstraint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	// Spec is the [ScalingConstraintSpec] used to generate scaling advice.
	// +required
	Spec ScalingConstraintSpec `json:"spec"`
	// Status contains validation and processing information for this ScalingConstraint.
	// +optional
	Status ScalingConstraintStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingConstraintList is a list of ScalingConstraint.
type ScalingConstraintList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	// Items is a slice of ScalingConstraint's.
	// +required
	Items []ScalingConstraint `json:"items"`
}

// ScalingConstraintSpec specifies the scaling constraints used to generate scaling advice.
type ScalingConstraintSpec struct {
	// SimulatorStrategy defines the simulator strategy used by the scaling planner.
	// +optional
	SimulatorStrategy apicommon.SimulatorStrategy `json:"simulatorStrategy"`
	// ScoringStrategy defines the node scoring strategy to use the [apicommon.SimulatorStrategySingleNodeMultiSim] '
	// strategy.
	// +optional
	ScoringStrategy apicommon.NodeScoringStrategy `json:"scoringStrategy"`
	// NodePools is the list of node pools to choose from when creating scaling advice.
	// +kubebuilder:validation:MinItems=1
	// +required
	NodePools []NodePool `json:"nodePools"`
}

// GetAllAvailabilityZones gets all the availability zones across all node pools as a sorted slice.
func (c *ScalingConstraintSpec) GetAllAvailabilityZones() []string {
	zoneSet := sets.NewString()
	for _, p := range c.NodePools {
		zoneSet.Insert(p.AvailabilityZones...)
	}
	zones := zoneSet.List()
	slices.Sort(zones)
	return zones
}

// ScalingConstraintStatus contains validation and processing information for this ScalingConstraint.
type ScalingConstraintStatus struct {
	// Conditions contains the conditions for the ScalingConstraint.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodePool defines a node pool configuration for a cluster.
type NodePool struct {
	// Labels is a map of key/value pairs for labels applied to all the nodes in this node pool.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations is a map of key/value pairs for annotations applied to all the nodes in this node pool.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Quota defines the quota for the node pool.
	// +optional
	Quota corev1.ResourceList `json:"quota,omitempty"`
	// Name is the name of the node pool. It must be unique within the cluster.
	// +required
	Name string `json:"name"`
	// Region is the name of the region.
	// +required
	Region string `json:"region"`
	// Taints is a list of taints applied to all the nodes in this node pool.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`
	// AvailabilityZones is a list of availability zones for the node pool.
	// +required
	AvailabilityZones []string `json:"availabilityZones"`
	// NodeTemplateRefs specifies the NodeTemplates that may be used for this NodePool together with their relative
	// priorities.
	// +kubebuilder:validation:MinItems=1
	// +required
	NodeTemplateRefs []NodeTemplateRef `json:"nodeTemplateRefs"`
	// Priority is the priority of the node pool.
	// +optional
	Priority int32 `json:"priority,omitzero"`
}

// NodeTemplateRef references a NodeTemplate and specifies its optional priority within the parent NodePool.
type NodeTemplateRef struct {
	// Name is the name of the referenced [NodeTemplate].  The referenced [NodeTemplate] must be defined in a [NodeTemplateSet].
	// +required
	Name string `json:"name"`
	// Priority defines the preference for this NodeTemplate when selecting a scale-out candidate in the parent NodePool.
	//
	// If omitted, the default implicit priority is zero. Higher values are preferred.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Priority int32 `json:"priority,omitzero"`
}
