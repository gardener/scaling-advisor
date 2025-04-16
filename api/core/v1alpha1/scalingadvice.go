// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={csa}

// ClusterScalingAdvice is the schema to define cluster scaling advice for a cluster.
type ClusterScalingAdvice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the specification of ClusterScalingAdvice.
	Spec ClusterScalingAdviceSpec `json:"spec"`
	// Status defines the status of ClusterScalingAdvice.
	Status ClusterScalingAdviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterScalingAdviceList is a list of ClusterScalingAdvice.
type ClusterScalingAdviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ClusterScalingAdvice.
	Items []ClusterScalingAdvice `json:"items"`
}

// ClusterScalingAdviceSpec defines the desired state of ClusterScalingAdvice.
type ClusterScalingAdviceSpec struct {
	// ConsumerID is the ID of the consumer who created the scaling constraints and is the target for cluster scaling advises.
	ConsumerID string `json:"consumerID"`
	// ScaleOutPlan is the plan for scaling out across node pools.
	ScaleOutPlan *ScaleOutPlan `json:"scaleOutPlan"`
	// ScaleInPlan is the plan for scaling in across node pools.
	ScaleInPlan *ScaleInPlan `json:"scaleInPlan"`
}

// ClusterScalingAdviceStatus defines the observed state of ClusterScalingAdvice.
type ClusterScalingAdviceStatus struct {
	// Backoffs contains the backoff information for each instance type + zone.
	Backoffs []ZoneInstanceTypeBackoff `json:"backoffs,omitempty"`
}

// ScaleOutPlan is the plan for scaling out a node pool.
type ScaleOutPlan struct {
	// Items is the slice of scaling-out advice for a node pool.
	Items []ScaleItem `json:"Items"`
}

// ScaleInPlan is the plan for scaling in a node pool and/or targeted set of nodes.
type ScaleInPlan struct {
	// Items is the slice of scaling-in advice for a node pool.
	Items []ScaleItem `json:"Items"`
	// NodeNames is the list of node names to be removed.
	NodeNames []string `json:"nodeNames"`
}

// ScaleItem is the unit of scaling advice for a node pool.
type ScaleItem struct {
	// NodePoolName is the name of the node pool.
	NodePoolName string `json:"nodePoolName"`
	// NodeTemplateName is the name of the node template.
	NodeTemplateName string `json:"nodeTemplateName"`
	// AvailabilityZone is the availability zone of the node pool.
	AvailabilityZone string `json:"availabilityZone"`
	// Delta is the delta change in the number of nodes for the node pool for this NodeTemplateName.
	Delta int32 `json:"delta"`
	// DesiredReplicas is the desired number of replicas for the node pool for this NodeTemplateName.
	DesiredReplicas int32 `json:"desiredReplicas"`
}

// ZoneInstanceTypeBackoff is the backoff information for each instance type + zone.
type ZoneInstanceTypeBackoff struct {
	// AvailabilityZone is the availability zone of the node pool.
	AvailabilityZone string `json:"availabilityZone"`
	// InstanceType is the instance type of the node pool.
	InstanceType string `json:"instanceType"`
	// FailCount is the number of nodes that have failed creation.
	FailCount int32 `json:"failCount"`
	// ObservedGeneration is the generation of the ClusterScalingAdvice for which this backoff was created.
	ObservedGeneration int64 `json:"observedGeneration"`
}
