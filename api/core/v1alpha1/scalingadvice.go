// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apicommon "github.com/gardener/scaling-advisor/api/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={sa}

// ScalingAdvice is the top-level object containing scaling advice produced by the scaling-advisor
// from a [ScalingConstraint] applied against a k8s cluster.
//
//nolint:govet // fieldalignment: intentional layout for k8s CRD / wire compatibility
type ScalingAdvice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the specification of ScalingAdvice.
	// +required
	Spec ScalingAdviceSpec `json:"spec"`
	// Status defines the observed state of ScalingAdvice.
	// +optional
	Status ScalingAdviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingAdviceList is a list of ScalingAdvice.
type ScalingAdviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ScalingAdvice.
	// +kubebuilder:validation:MinItems=1
	// +required
	Items []ScalingAdvice `json:"items"`
}

// ScalingAdviceSpec contains the generated scaling advice which holds [ScaleOut] and/or [ScaleIn] plans.
type ScalingAdviceSpec struct {
	// ScaleOut is the plan for scaling out across node pools.
	// +optional
	ScaleOut *ScaleOutPlan `json:"scaleOut,omitempty"`
	// ScaleIn is the plan for scaling in across node pools.
	// +optional
	ScaleIn *ScaleInPlan `json:"scaleIn,omitempty"`
	// Diagnostic provides diagnostics information for the scaling advice.
	// This is only set by the scaling advisor controller if the constants.AnnotationEnableScalingDiagnostics annotation is
	// set on the corresponding ScalingConstraint resource.
	// +optional
	Diagnostic *ScalingAdviceDiagnostic `json:"diagnostic,omitempty"`
	// ConstraintRef references the [ScalingConstraint] for which this [ScalingAdvice] was generated.
	// +required
	ConstraintRef apicommon.NamespacedName `json:"constraintRef"`
}

// ScalingAdviceStatus contains the observed status from the lifecycle manager (external actor which consumes the
// [ScalingAdviceSpec]. It holds the [ScalingFeedback] and [metav1.Condition]'s.
type ScalingAdviceStatus struct {
	// Feedback represents the [ScalingFeedback] from the lifecycle manager applying the [ScalingAdvice]
	// +optional
	Feedback *ScalingFeedback `json:"feedback,omitempty"`
	// Conditions contains the [metav1.Condition]'s for this ScalingAdvice.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ScaleOutPlan is the plan for scaling out a node pool.
type ScaleOutPlan struct {
	// UnsatisfiedPodNames contains pod names in "namespace/name" format that could not be scheduled by this scale-out
	// plan.
	UnsatisfiedPodNames []string `json:"unsatisfiedPodNames,omitempty"`
	// Items is the slice of scaling-out advice for a node pool.
	// +kubebuilder:validation:MinItems=1
	// +required
	Items []ScaleOutItem `json:"items"`
}

// ScaleInPlan is the plan for scaling in a node pool and/or targeted set of nodes.
type ScaleInPlan struct {
	// Items is the slice of scaling-in advice for a node pool.
	// +kubebuilder:validation:MinItems=1
	// +required
	Items []ScaleInItem `json:"items"`
}

// ScaleInItem is the unit of scaling-in advice for a specific node.
type ScaleInItem struct {
	NodePlacement `json:",inline"`
	// NodeName is the name of the node to be scaled in.
	// +required
	NodeName string `json:"nodeName"`
}

// ScaleOutItem is the unit of scaling advice for a node pool.
type ScaleOutItem struct {
	NodePlacement `json:",inline"`
	// CurrentReplicas is the current number of replicas for the NodePlacement.
	// +required
	CurrentReplicas int32 `json:"currentReplicas"`
	// Delta is the delta change in the number of nodes for the NodePlacement.
	// +kubebuilder:validation:Minimum=1
	// +required
	Delta int32 `json:"delta"`
}

// NodePlacement identifies the target node pool, node template, and cloud location associated with a scaling operation.
type NodePlacement struct {
	// PoolName is the name of the node pool.
	// +required
	PoolName string `json:"poolName"`
	// TemplateName is the name of the node template.
	// +required
	TemplateName string `json:"templateName"`
	// InstanceType is the instance type of the Node
	// +required
	InstanceType string `json:"instanceType"`
	// Region is the region of the instance
	// +required
	Region string `json:"region"`
	// AvailabilityZone is the availability zone of the node pool.
	// +required
	AvailabilityZone string `json:"availabilityZone"`
}

// ScalingAdviceDiagnostic provides diagnostics information for the scaling advice.
type ScalingAdviceDiagnostic struct {
	// TraceLogName is the name of the trace log. This can be used to fetch the trace log.
	TraceLogName string `json:"traceLogName"`
	// RunResultsLogName is the name of the run resets log.  This can be used to fetch the run results log.
	RunResultsLogName string `json:"runResultsLogName"`
}

// ScalingFeedback provides scale-in and scale-out feedback from the lifecycle manager.
// Scaling advisor can refine its future scaling advice based on this feedback.
type ScalingFeedback struct {
	// ScaleOut is the scale-out feedback from the lifecycle manager when applying [ScaleOutPlan]
	// [ScalingAdviceSpec].
	ScaleOut *ScaleOutFeedback `json:"scaleOut,omitempty"`
	// ScaleIn is the scale-in feedback from life-cycle manager when applying [ScaleInPlan]
	ScaleIn *ScaleInFeedback `json:"scaleIn,omitempty"`
}

// ScalingErrorType defines the type of scaling error.
//
// +kubebuilder:validation:Enum=ResourceExhaustedError;CreationTimeoutError
type ScalingErrorType string

const (
	// ScalingErrorTypeResourceExhausted indicates that the lifecycle manager could not create the instance due to resource exhaustion for an instance type in an availability zone.
	ScalingErrorTypeResourceExhausted ScalingErrorType = "ResourceExhaustedError"
	// ScalingErrorTypeCreationTimeout indicates that the lifecycle manager could not create the instance within its configured timeout despite multiple attempts.
	ScalingErrorTypeCreationTimeout ScalingErrorType = "CreationTimeoutError"
)

// ScaleOutFeedback is the feedback from the life cycle manager when applying an [ScaleOutPlan] of a [ScalingAdviceSpec]
type ScaleOutFeedback struct {
	// Items contains item feedback corresponding to [ScaleOutItem]. This field must be non-empty whenever
	// [ScaleOutFeedback] is present.
	// +kubebuilder:validation:MinItems=1
	// +required
	Items []ScaleOutItemFeedback `json:"items"`
}

// ScaleOutItemFeedback is the feedback from the life cycle manager when applying an individual [ScaleOutItem].
type ScaleOutItemFeedback struct {
	// CreationDeadline represents the time after which the scaling-advisor can expect real nodes to be created and available
	// for the corresponding [ScaleOutItem]'s [NodePlacement]. When the [ScalingFeedback] is constructed by the life-cycle manager,
	// this field is mandatory to be set inside all [ScaleOutItemFeedback]
	// +required
	CreationDeadline metav1.Time `json:"creationDeadline"`
	// BackoffUntil if populated, represents the time until the scaling-advisor will not consider the corresponding
	// [ScaleOutItem]'s [NodePlacement] when running simulations and generating subsequent [ScaleOutPlan]'s
	// +optional
	BackoffUntil *metav1.Time `json:"backoffUntil,omitempty"`
	// ErrorType is the type of error that occurred during scale-out.
	// +optional
	ErrorType ScalingErrorType `json:"errorType,omitempty"`
	// Index represents the item index in [ScaleOutPlan.Items]
	// +required
	Index int32 `json:"index"`
	// FailCount is the number of nodes that have failed creation.
	// +optional
	FailCount int32 `json:"failCount,omitzero"`
}

// ScaleInFeedback is  the feedback from the life cycle manager after applying [ScaleInPlan]
type ScaleInFeedback struct {
	// AcceptedNodeNames holds the slice of node names that were accepted for scale-in by the lifecycle controller.
	// This field must be non-empty whenever [ScaleInFeedback] is present.
	// +kubebuilder:validation:MinItems=1
	// +required
	AcceptedNodeNames []string `json:"acceptedNodeNames"`
}
