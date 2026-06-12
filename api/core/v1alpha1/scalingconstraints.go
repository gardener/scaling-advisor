// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={sc}

// ScalingConstraint is a schema to define constraints that will be used to create cluster scaling advises for a cluster.
type ScalingConstraint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`
	// Spec defines the specification of the ScalingConstraint.
	Spec ScalingConstraintSpec `json:"spec"`
	// Status defines the status of the ScalingConstraint.
	Status ScalingConstraintStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingConstraintList is a list of ScalingConstraint.
type ScalingConstraintList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ScalingConstraint's.
	Items []ScalingConstraint `json:"items"`
}

// ScalingConstraintSpec defines the specification of the ScalingConstraint.
type ScalingConstraintSpec struct {
	// NodePools is the list of node pools to choose from when creating scaling advice.
	NodePools []NodePool `json:"nodePools,omitempty"`
	// NodeTemplates is the slice of all NodeTemplates used within the scaling constraint spec.
	NodeTemplates []NodeTemplate `json:"nodeTemplates"`
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

// ScalingConstraintStatus defines the observed state of ScalingConstraint.
type ScalingConstraintStatus struct {
	// Conditions contains the conditions for the ScalingConstraint.
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
	AvailabilityZones []string `json:"availabilityZones"`
	// Requirements encapsulates the slice of requirement selectors for this NodePool
	// +optional
	Requirements []NodePoolRequirement `json:"requirements,omitempty"`
	// Priority is the priority of the node pool.
	// +optional
	Priority int32 `json:"priority,omitzero"`
}

// NodePoolRequirement is a requirement selector that encapsulates values, a key, and an operator
// that relates the key and values.
type NodePoolRequirement struct {
	// Key is the label key that the selector applies to.
	// +required
	Key string `json:"key"`
	// Operator represents a key's relationship to a set of values.
	// Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt.
	// +required
	Operator NodePoolRequirementOperator `json:"operator"`
	// Values is an array of string values. If the operator is "In" or "NotIn",
	// the values array must be non-empty. If the operator is "Exists" or "DoesNotExist:,
	// the values array must be empty. If the operator is "Gt" or "Lt", the values
	// array must have a single element, which will be interpreted as an integer.
	// This array is replaced during a strategic merge patch.
	// +optional
	// +listType=atomic
	Values []string `json:"values,omitempty"`
	// Priority represents the priority of this requirement. Higher values have greater priority.
	// +optional
	Priority int32 `json:"priority,omitzero"`
}

// NodePoolRequirementOperator is the set of operators that can be used in a [NodePoolRequirement]
// +enum
type NodePoolRequirementOperator string

const (
	// NodePoolRequirementOpIn is the enum constant for the "In" operator used within a [NodePoolRequirement].
	NodePoolRequirementOpIn NodePoolRequirementOperator = "In"
	// NodePoolRequirementOpNotIn is the enum constant for the "NotIn" operator used within a [NodePoolRequirement].
	NodePoolRequirementOpNotIn NodePoolRequirementOperator = "NotIn"
	// NodePoolRequirementOpExists is the enum constant for the "Exist" operator used within a [NodePoolRequirement].
	NodePoolRequirementOpExists NodePoolRequirementOperator = "Exists"
	// NodePoolRequirementOpDoesNotExist is the enum constant for the "DoesNotExist" operator used within a [NodePoolRequirement].
	NodePoolRequirementOpDoesNotExist NodePoolRequirementOperator = "DoesNotExist"
	// NodePoolRequirementOpGt is the enum constant for the "Gt" operator used within a [NodePoolRequirement].
	NodePoolRequirementOpGt NodePoolRequirementOperator = "Gt"
	// NodePoolRequirementOpLt is the enum constant for the "Lt" operator used within a [NodePoolRequirement].
	NodePoolRequirementOpLt NodePoolRequirementOperator = "Lt"
)

// NodeTemplate defines a node template configuration for an instance type.
// There can be different NodeTemplate's for a [ScalingConstraintSpec] for the same instance type.
// This is permitted to allow the opportunity for different SystemReserved.
type NodeTemplate struct {
	// Capacity defines the capacity of resources that are available for this instance type.
	Capacity corev1.ResourceList `json:"capacity"`
	// KubeReserved defines the capacity for kube reserved resources.
	// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#kube-reserved for additional information.
	// +optional
	KubeReserved corev1.ResourceList `json:"kubeReservedCapacity,omitempty"`
	// SystemReserved defines the capacity for system reserved resources.
	// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#system-reserved for additional information.
	// Please read https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#general-guidelines when deciding to
	// +optional
	SystemReserved corev1.ResourceList `json:"systemReservedCapacity,omitempty"`
	// Name is the name of the node template. Name is unique within a particular [ScalingConstraintSpec]
	Name string `json:"name"`
	// Architecture is the architecture of the instance type.
	Architecture string `json:"architecture"`
	// InstanceType is the instance type of the node template.
	InstanceType string `json:"instanceType"`
	// MaxVolumes is the max number of volumes that can be attached to a node of this instance type.
	MaxVolumes int32 `json:"maxVolumes,omitzero"`
}

// InstancePricing contains the pricing information for an instance type.
type InstancePricing struct {
	// UnitCPUPrice is the price per CPU of the instance type.
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Format=double
	UnitCPUPrice *float64 `json:"unitCPUPrice,omitempty"`
	// UnitMemoryPrice is the price per memory of the instance type.
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Format=double
	UnitMemoryPrice *float64 `json:"unitMemoryPrice,omitempty"`
	// InstanceType is the instance type of the node template.
	InstanceType string `json:"instanceType"`
	// Price is the total price of the instance type.
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Format=double
	Price float64 `json:"price"`
}
