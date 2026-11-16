package v1alpha1

import (
	apicommon "github.com/gardener/scaling-advisor/api/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName={ids},scope=Namespaced
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// InstanceTypeDefSet defines a top-level object whose spec contains all [InstanceTypeDef]'s known to the
// scaling-advisor. If an instance type is referenced by a [NodeTemplate] or an [InstancePrice], then
// a corresponding [InstanceTypeDef] must exist in the [InstanceTypeDefSet].
type InstanceTypeDefSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the specification of this InstanceTypeDefSet.
	// +required
	Spec InstanceTypeDefSetSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InstanceTypeDefSetList is a list of InstanceTypeDefSet
type InstanceTypeDefSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of InstanceTypeDefSet's.
	// +required
	Items []InstanceTypeDefSet `json:"items"`
}

// InstanceTypeDefSetSpec contains a slice of [InstanceTypeDef]'s.
type InstanceTypeDefSetSpec struct {
	// Provider is the cloud provider offering these instance types
	// +required
	Provider apicommon.CloudProvider `json:"provider"`
	// InstanceTypeDefs is a slice of [InstanceTypeDef]'s. InstanceTypeDef names should be unique.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	// +required
	InstanceTypeDefs []InstanceTypeDef `json:"instanceTypeDefs"`
}

// InstanceTypeDef holds core instance information about an instance type offered by a cloud provider.
type InstanceTypeDef struct {
	// BaseResources specifies the base, natively-available resources for this instance type as offered by the cloud provider.
	// +required
	BaseResources corev1.ResourceList `json:"baseResources"`
	// Architecture is the cpu architecture.
	// +kubebuilder:validation:MinLength=1
	// +required
	Architecture string `json:"architecture"`
	// Name is the name of the cloud provider's instance type. (Ex: m5.xlarge, etc). This should be unique within a [InstanceTypeDefSet]
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
}
