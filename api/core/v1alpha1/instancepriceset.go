package v1alpha1

import (
	apicommon "github.com/gardener/scaling-advisor/api/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName={ips}

// InstancePriceSet defines a top-level object with a [InstancePriceSetSpec] consisting of all the [InstancePrice]'s
// that are referenced by any [NodeTemplate].
type InstancePriceSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`
	// Spec defines the specification of this InstancePriceSet
	// +required
	Spec InstancePriceSetSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InstancePriceSetList contains a list of InstancePriceSet.
type InstancePriceSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of [InstancePriceSet].
	// +required
	Items []InstancePriceSet `json:"items"`
}

// InstancePriceSetSpec contains a list of [InstancePrice]'s that is semantically a set
// where each [InstancePriceKey] is unique.
type InstancePriceSetSpec struct {
	// Prices is a slice of [InstancePrice]'s which should be set uniquely keyed on
	// [InstancePrice.InstanceType], [InstancePrice.CapacityType] and [InstancePrice.Region].
	// +kubebuilder:validation:MinItems=1
	// +required
	Prices []InstancePrice `json:"prices"`
}

// InstancePriceKey uniquely identifies an [InstancePrice].
//
// The combination of [InstancePriceKey.InstanceType], [InstancePriceKey.Region], and [InstancePriceKey.CapacityType]
// must be unique within an [InstancePriceSet].
type InstancePriceKey struct {
	// InstanceType is the instance type of the node template.
	// +required
	InstanceType string `json:"instanceType"`
	// Region is the cloud provider region.
	// +required
	Region string `json:"region"`
	// CapacityType represents procurement model of compute capacity.
	// +required
	CapacityType apicommon.CapacityType `json:"capacityType"`
}

// InstancePrice describes pricing information along with an [InstancePriceKey].
type InstancePrice struct {
	// UnitCPUPrice is the price per CPU of the instance type.
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Format=double
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:ExclusiveMinimum=true
	// +optional
	UnitCPUPrice *float64 `json:"unitCPUPrice,omitempty"`
	// UnitMemoryPrice is the price per memory of the instance type.
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Format=double
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:ExclusiveMinimum=true
	// +optional
	UnitMemoryPrice  *float64 `json:"unitMemoryPrice,omitempty"`
	InstancePriceKey `json:",inline"`
	// Price is the total price of the instance type.
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Format=double
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:ExclusiveMinimum=true
	// +required
	Price float64 `json:"price"`
}
