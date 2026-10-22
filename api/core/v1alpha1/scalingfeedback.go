// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName={sfb},scope=Namespaced
// +kubebuilder:printcolumn:name="Acknowledged",type=date,JSONPath=`.spec.acknowledgedAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ScalingFeedback is written by the lifecycle manager in response to a [ScalingAdvice].
// It is owned by the corresponding [ScalingAdvice] via an owner reference and is
// garbage-collected when the [ScalingAdvice] is deleted.
//
// Field manager ownership:
//   - The lifecycle manager owns all fields in [ScalingFeedbackSpec].
//   - The scaling-advisor must not write to this resource.
//
// Linking convention: a ScalingFeedback shares the same name and namespace as its
// owning [ScalingAdvice], making lookup a direct Get rather than a List+filter.
//
//nolint:govet // fieldalignment: intentional layout for k8s CRD / wire compatibility
type ScalingFeedback struct {
	Spec              ScalingFeedbackSpec `json:"spec"`
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingFeedbackList is a list of ScalingFeedback.
type ScalingFeedbackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ScalingFeedback.
	// +required
	Items []ScalingFeedback `json:"items"`
}

// ScalingFeedbackSpec contains the feedback from the lifecycle manager regarding a [ScalingAdvice].
type ScalingFeedbackSpec struct {
	// AcknowledgedAt is set by the lifecycle manager to the time at which it observed this
	// [ScalingAdvice] and committed to executing the full scale-out/scale-in plan.
	// The scaling-advisor will not produce a subsequent [ScalingAdvice] until this field is set.
	// +optional
	AcknowledgedAt *metav1.Time `json:"acknowledgedAt,omitempty"`
	// ScaleOut is the scale-out feedback from the lifecycle manager when applying [ScaleOutPlan].
	// +optional
	ScaleOut *ScaleOutFeedback `json:"scaleOut,omitempty"`
	// ScaleIn is the scale-in feedback from the lifecycle manager when applying [ScaleInPlan].
	// +optional
	ScaleIn *ScaleInFeedback `json:"scaleIn,omitempty"`
}

// ScaleOutFeedback is the feedback from the lifecycle manager when applying a [ScaleOutPlan].
type ScaleOutFeedback struct {
	// Items contains item feedback corresponding to each [ScaleOutItem]. This field must be non-empty
	// whenever [ScaleOutFeedback] is present.
	// +kubebuilder:validation:MinItems=1
	// +required
	Items []ScaleOutItemFeedback `json:"items"`
}

// ScaleOutItemFeedback is the feedback from the lifecycle manager when applying an individual [ScaleOutItem].
type ScaleOutItemFeedback struct {
	// BackoffUntil if populated, represents the time until the scaling-advisor will not consider the
	// corresponding [ScaleOutItem]'s [NodePlacement] when running simulations and generating subsequent
	// [ScaleOutPlan]'s.
	// +optional
	BackoffUntil *metav1.Time `json:"backoffUntil,omitempty"`
	// Message is a human-readable log message from the lifecycle manager regarding this scale-out item.
	// +optional
	Message string `json:"message,omitempty"`
	// Name is the name of the corresponding [ScaleOutItem] in [ScaleOutPlan.Items].
	// +required
	Name string `json:"name"`
	// FailCount is the number of nodes that have failed creation. The scaling-advisor uses this to
	// decide whether to retain or discard [NodePlacement]'s for this item.
	// +required
	FailCount int32 `json:"failCount"`
}

// ScaleInFeedback is the feedback from the lifecycle manager after applying a [ScaleInPlan].
type ScaleInFeedback struct {
	// AcceptedNodeNames holds the node names accepted for scale-in by the lifecycle controller.
	// This field must be non-empty whenever [ScaleInFeedback] is present.
	// +kubebuilder:validation:MinItems=1
	// +required
	AcceptedNodeNames []string `json:"acceptedNodeNames"`
}
