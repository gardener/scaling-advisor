// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apicommon "github.com/gardener/scaling-advisor/api/common/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	// ScaleOutPlan is the plan for scaling out across node pools.
	// +optional
	ScaleOutPlan *ScaleOutPlan `json:"scaleOutPlan"`
	// ScaleInPlan is the plan for scaling in across node pools.
	ScaleInPlan *ScaleInPlan `json:"scaleInPlan"`
	// ConstraintRef is a reference to the ClusterScalingConstraint that this advice is based on.
	ConstraintRef apicommon.ConstraintReference `json:"constraintRef"`
}

// ClusterScalingAdviceStatus defines the observed state of ClusterScalingAdvice.
type ClusterScalingAdviceStatus struct {
	// Diagnostic provides diagnostics information for the scaling advice.
	// This is only set by the scaling advisor controller if the constants.AnnotationEnableScalingDiagnostics annotation is
	// set on the corresponding ClusterScalingConstraint resource.
	// +optional
	Diagnostic *ScalingAdviceDiagnostic `json:"diagnostic,omitempty"`
	// Conditions represents additional information
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ScaleOutPlan is the plan for scaling out a node pool.
type ScaleOutPlan struct {
	// UnsatisfiedPodNames is the list of all pods (namespace/name) that could not be satisfied by the scale out plan.
	UnsatisfiedPodNames []string `json:"unsatisfiedPodNames"`
	// Items is the slice of scaling-out advice for a node pool.
	Items []ScaleOutItem `json:"Items"`
}

// ScaleInPlan is the plan for scaling in a node pool and/or targeted set of nodes.
type ScaleInPlan struct {
	// Items is the slice of scaling-in advice for a node pool.
	Items []ScaleInItem `json:"items"`
}

// ScaleInItem is the unit of scaling-in advice for a specific node.
type ScaleInItem struct {
	NodePlacement `json:",inline"`
	// NodeName is the name of the node to be scaled in.
	NodeName string `json:"nodeName"`
}

// ScaleOutItem is the unit of scaling advice for a node pool.
type ScaleOutItem struct {
	NodePlacement `json:",inline"`
	// CurrentReplicas is the current number of replicas for the NodePlacement.
	CurrentReplicas int32 `json:"currentReplicas"`
	// Delta is the delta change in the number of nodes for the NodePlacement.
	Delta int32 `json:"delta"`
}

// NodePlacement provides information about the placement of a node.
type NodePlacement struct {
	// NodePoolName is the name of the node pool.
	NodePoolName string `json:"nodePoolName"`
	// NodeTemplateName is the name of the node template.
	NodeTemplateName string `json:"nodeTemplateName"`
	// InstanceType is the instance type of the NodeResources.
	InstanceType string `json:"instanceType"`
	// Region is the region of the instance
	Region string `json:"region"`
	// AvailabilityZone is the availability zone of the node pool.
	AvailabilityZone string `json:"availabilityZone"`
}

// ScalingAdviceDiagnostic provides diagnostics information for the scaling advice.
type ScalingAdviceDiagnostic struct {
	// TraceLogName is the name of the trace log. This can be used to fetch the trace log from the scaling advisor core.
	TraceLogName string `json:"traceLogName"`
	// SimRunResults is the list of simulation run results for the scaling advice.
	SimRunResults []ScalingSimRunResult `json:"simRunResults"`
}

// ScalingSimRunResult is the result of a simulation run in the scaling advisor.
type ScalingSimRunResult struct {
	// NodePoolName is the name of the node pool.
	NodePoolName string `json:"nodePoolName"`
	// NodeTemplateName is the name of the node template.
	NodeTemplateName string `json:"nodeTemplateName"`
	// AvailabilityZone is the availability zone of the node pool.
	AvailabilityZone string `json:"availabilityZone"`
	// ScheduledPodNames is the list of pod names that were scheduled in this simulation run.
	ScheduledPodNames []string `json:"scheduledPodNames"`
	// NodeScore is the score of the node in the simulation run.
	NodeScore int64 `json:"nodeScore"`
	// NumUnscheduledPods is the number of pods that could not be scheduled in this simulation run.
	NumUnscheduledPods int32 `json:"numUnscheduledPods"`
}
