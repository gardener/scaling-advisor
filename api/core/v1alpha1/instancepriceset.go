package v1alpha1

import (
	apicommon "github.com/gardener/scaling-advisor/api/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName={ips},scope=Namespaced
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// InstancePriceSet defines a top-level object with a [InstancePriceSetSpec] consisting of all the [InstancePrice]'s.
// Any instance type referenced by a [NodeTemplate] should have a corresponding [InstancePrice].
type InstancePriceSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
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
	// Provider is the cloud provider for which these prices apply.
	// +required
	Provider apicommon.CloudProvider `json:"provider"`
	// Prices is a slice of [InstancePrice]'s uniquely keyed on the composite
	// ([InstancePrice.InstanceType], [InstancePrice.Region], [InstancePrice.CapacityType]) tuple.
	// Duplicate composite keys are rejected by admission validation.
	//
	// +listType=map
	// +listMapKey=instanceType
	// +listMapKey=region
	// +listMapKey=capacityType
	// +kubebuilder:validation:MinItems=1
	// +required
	Prices []InstancePrice `json:"prices"`
}

// InstancePriceKey uniquely identifies an [InstancePrice].
//
// The combination of [InstancePriceKey.InstanceType], [InstancePriceKey.Region], and [InstancePriceKey.CapacityType]
// must be unique within an [InstancePriceSet].
type InstancePriceKey struct {
	// InstanceType is the name of the cloud provider's instance type.
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
//
//nolint:govet // fieldalignment: intentional layout for k8s CRD / wire compatibility
type InstancePrice struct {
	InstancePriceKey  `json:",inline"`
	Price             string              `json:"price"`
	ResourceUnitPrice []ResourceUnitPrice `json:"resourceUnitPrice,omitempty"`
}

// ResourceUnitPrice is the unit price for a resource.
type ResourceUnitPrice struct {
	// +required
	Name corev1.ResourceName `json:"name"`
	// Value is the price per unit of this resource in USD, expressed as a positive decimal string.
	// The value must be greater than zero.
	// Examples: "0.048", "0.006"
	//
	// Using string rather than float64 follows the convention established by other Kubernetes ecosystem
	// projects to avoid floating-point representation issues in CRD-serialized fields:
	//   - karpenter NodeOverlaySpec.Price *string (github.com/kubernetes-sigs/karpenter, pkg/apis/v1alpha1/nodeoverlay.go)
	//   - cluster-api-provider-aws SpotMarketOptions.MaxPrice *string (sigs.k8s.io/cluster-api-provider-aws, api/v1beta2/types.go)
	//   - kubecost OTC cloud types Price string (github.com/opencost/opencost, pkg/cloud/otc/types.go)
	//
	// +kubebuilder:validation:Pattern=`^(0\.[0-9]*[1-9]|[1-9][0-9]*(\.[0-9]+)?)$`
	// +required
	Value string `json:"value"`
}
