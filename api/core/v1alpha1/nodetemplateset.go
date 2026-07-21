package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName={nts}

// NodeTemplateSet defines top-level object with a [NodeTemplateSetSpec] holding all the [NodeTemplate]'s known
// to the scaling-advisor. Any NodeTemplate referenced in a [NodePool] within a [ScalingConstraintSpec] must have a
// corresponding [NodeTemplate] object within the data plane.
type NodeTemplateSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`
	// Spec defines the specification of the NodeTemplateSet
	// +required
	Spec NodeTemplateSetSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeTemplateSetList is a list of NodeTemplateSet
type NodeTemplateSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of NodeTemplateSet's.
	// +required
	Items []NodeTemplateSet `json:"items"`
}

// NodeTemplateSetSpec contains a slice of [NodeTemplate]'s
type NodeTemplateSetSpec struct {
	// Templates is a slice of [NodeTemplate]'s. NodeTemplate names should be unique.
	// +kubebuilder:validation:MinItems=1
	// +required
	Templates []NodeTemplate `json:"templates"`
}

// NodeTemplate defines a node template configuration for an instance type.
// Multiple NodeTemplates may reference the same instance type which permits different reserved resource configurations
// for the same underlying instance type. However, node template names are expected to be unique within a [NodeTemplateSet].
type NodeTemplate struct {
	// Capacity defines the capacity of resources that are available for this instance type.
	// +required
	Capacity corev1.ResourceList `json:"capacity"`
	// Reserved defines the capacity for total reserved resources. This should include both system and Kubernetes components.
	// +optional
	Reserved corev1.ResourceList `json:"reservedCapacity,omitempty"`
	// Name is a logical name given to this NodeTemplate specification
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
	// Architecture is the architecture of the instance type.
	// +kubebuilder:validation:MinLength=1
	// +required
	Architecture string `json:"architecture"`
	// InstanceType is the instance type of the node template.
	// +kubebuilder:validation:MinLength=1
	// +required
	InstanceType string `json:"instanceType"`
	// MaxVolumes is the max number of volumes that can be attached to a node of this instance type.
	// +kubebuilder:validation:Minimum=0
	// +required
	MaxVolumes int32 `json:"maxVolumes,omitzero"`
}
