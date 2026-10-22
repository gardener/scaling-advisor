package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName={nts},scope=Namespaced

// NodeTemplateSet defines a top-level object whose spec contains all [NodeTemplate]s known to the scaling-advisor.
//
// Any NodeTemplate referenced by a [NodePool] in a [ScalingConstraintSpec] must exist in the NodeTemplateSet.
type NodeTemplateSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
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
	// +listType=map
	// +listMapKey=name
	// +required
	Templates []NodeTemplate `json:"templates"`
}

// NodeTemplate defines a node template configuration for an instance type. Multiple NodeTemplates may reference the
// same instance type which permits different reserved resource configurations (and scaling limits?)
// for the same underlying instance type.
// However, node template names are expected to be unique within a [NodeTemplateSet].
type NodeTemplate struct {
	// ExtendedResources specifies Kubernetes extended resources advertised by nodes created from this template, over
	// and above the instance type's BaseResources.
	// +optional
	ExtendedResources corev1.ResourceList `json:"extendedResources,omitempty"`
	// ReservedResources specifies resources reserved for the operating system, Kubernetes components, and other node-level
	// services. These resources are not available for scheduling workloads.
	// +required
	ReservedResources corev1.ResourceList `json:"reservedResources"`
	// InstanceType is the instance type of the node template. Should be associated with some [InstanceTypeDef].
	// +kubebuilder:validation:MinLength=1
	// +required
	InstanceType string `json:"instanceType"`
	// Name is a logical name given to this NodeTemplate specification
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
}
