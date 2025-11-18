// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={csc}

// ClusterScalingConstraint is a schema to define constraints that will be used to create cluster scaling advises for a cluster.
type ClusterScalingConstraint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the specification of the ClusterScalingConstraint.
	Spec ClusterScalingConstraintSpec `json:"spec"`
	// Status defines the status of the ClusterScalingConstraint.
	Status ClusterScalingConstraintStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterScalingConstraintList is a list of ClusterScalingConstraint.
type ClusterScalingConstraintList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ClusterScalingConstraint's.
	Items []ClusterScalingConstraint `json:"items"`
}

// ClusterScalingConstraintSpec defines the specification of the ClusterScalingConstraint.
type ClusterScalingConstraintSpec struct {
	// DefaultBackoffPolicy defines a default backoff policy for all NodePools of a cluster. Backoff policy can be overridden at the NodePool level.
	// +optional
	DefaultBackoffPolicy *BackoffPolicy `json:"defaultBackoffPolicy"`
	// ScaleInPolicy defines the default scale in policy to be used when scaling in a node pool.
	// +optional
	ScaleInPolicy *ScaleInPolicy `json:"scaleInPolicy"`
	// ConsumerID is the ID of the consumer who creates the scaling constraint and is the target for cluster scaling advises.
	// It allows a consumer to accept or reject the advises by checking the ConsumerID for which the scaling advice has been created.
	ConsumerID string `json:"consumerID"`
	// AdviceGenerationMode defines the mode in which scaling advice is generated.
	AdviceGenerationMode ScalingAdviceGenerationMode `json:"adviceGenerationMode"`
	// NodePools is the list of node pools to choose from when creating scaling advice.
	NodePools []NodePool `json:"nodePools"`
}

// ClusterScalingConstraintStatus defines the observed state of ClusterScalingConstraint.
type ClusterScalingConstraintStatus struct {
	// Conditions contains the conditions for the ClusterScalingConstraint.
	Conditions []metav1.Condition `json:"conditions"`
}

// ScalingAdviceGenerationMode defines the mode in which scaling advice is generated.
type ScalingAdviceGenerationMode string

const (
	// ScalingAdviceGenerationModeIncremental is the mode in which scaling advice is generated incrementally.
	// In this mode, scaling advisor will dish out scaling advice as soon as it has the first scale-out/in advice from a simulation run.
	ScalingAdviceGenerationModeIncremental = "incremental"
	// ScalingAdviceGenerationModeAllAtOnce is the mode in which scaling advice is generated all at once.
	// In this mode, scaling advisor will generate scaling advice after it has run the complete set of simulations wher either
	// all pending pods have been scheduled or stabilised.
	ScalingAdviceGenerationModeAllAtOnce = "all-at-once"
)

var (
	// AllScalingAdviceGenerationModes is the set of all ScalingAdviceGenerationMode's
	AllScalingAdviceGenerationModes = sets.New[ScalingAdviceGenerationMode](ScalingAdviceGenerationModeIncremental, ScalingAdviceGenerationModeAllAtOnce)
)

// NodePool defines a node pool configuration for a cluster.
type NodePool struct {
	// Labels is a map of key/value pairs for labels applied to all the nodes in this node pool.
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations is a map of key/value pairs for annotations applied to all the nodes in this node pool.
	Annotations map[string]string `json:"annotations,omitempty"`
	// Quota defines the quota for the node pool.
	Quota corev1.ResourceList `json:"quota,omitempty"`
	// ScaleInPolicy defines the scale in policy for this node pool.
	// +optional
	ScaleInPolicy *ScaleInPolicy `json:"scaleInPolicy,omitempty"`
	// BackoffPolicy defines the backoff policy applicable to resource exhaustion of any instance type + zone combination in this node pool.
	BackoffPolicy *BackoffPolicy `json:"defaultBackoffPolicy,omitempty"`
	// Name is the name of the node pool. It must be unique within the cluster.
	Name string `json:"name"`
	// Region is the name of the region.
	Region string `json:"region"`
	// Taints is a list of taints applied to all the nodes in this node pool.
	Taints []corev1.Taint `json:"taints,omitempty"`
	// AvailabilityZones is a list of availability zones for the node pool.
	AvailabilityZones []string `json:"availabilityZones"`
	// NodeTemplates is a slice of NodeTemplate.
	NodeTemplates []NodeTemplate `json:"nodeTemplates"`
	// Priority is the priority of the node pool.
	Priority int32 `json:"priority"`
}

// NodeTemplate defines a node template configuration for an instance type.
// All nodes of a certain instance type in a node pool will be created using this template.
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
	// Name is the name of the node template.
	Name string `json:"name"`
	// Architecture is the architecture of the instance type.
	Architecture string `json:"architecture"`
	// InstanceType is the instance type of the node template.
	InstanceType string `json:"instanceType"`
	// Priority is the priority of the node template. The lower the number, the higher the priority.
	Priority int32 `json:"priority"`
	// MaxVolumes is the max number of volumes that can be attached to a node of this instance type.
	MaxVolumes int32 `json:"maxVolumes"`
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

// BackoffPolicy defines the backoff policy to be used when backing off from suggesting an instance type + zone in subsequence scaling advice upon failed scaling operation.
type BackoffPolicy struct {
	// InitialBackoffDuration defines the lower limit of the backoff duration.
	InitialBackoffDuration metav1.Duration `json:"initialBackoff"`
	// MaxBackoffDuration defines the upper limit of the backoff duration.
	MaxBackoffDuration metav1.Duration `json:"maxBackoff"`
}

// ScaleInPolicy defines the scale in policy to be used when scaling in a node pool.
type ScaleInPolicy struct {
	//TODO design this better.
}
