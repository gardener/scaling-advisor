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
// +kubebuilder:resource:shortName={sf}

// ScalingFeedback provides scale-in and scale-out error feedback from the lifecycle manager.
// Scaling advisor can refine its future scaling advice based on this feedback.
type ScalingFeedback struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the specification of ScalingFeedback.
	Spec ScalingFeedbackSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingFeedbackList is a list of ScalingFeedback.
type ScalingFeedbackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a slice of ScalingFeedback.
	Items []ScalingFeedback `json:"items"`
}

// ScalingFeedbackSpec defines the specification of the ScalingFeedback.
type ScalingFeedbackSpec struct {
	// ConstraintRef is a reference to the ScalingConstraint that this advice is based on.
	ConstraintRef apicommon.ConstraintReference `json:"constraintRef"`
	// ScaleOutErrorInfos is the list of scale-out errors for the scaling advice.
	ScaleOutErrorInfos []ScaleOutErrorInfo `json:"scaleOutErrorInfos,omitempty"`
	// ScaleInErrorInfo is the scale-in error information for the scaling advice.
	ScaleInErrorInfo ScaleInErrorInfo `json:"scaleInErrorInfo,omitempty"`
}

// ScalingErrorType defines the type of scaling error.
// +enum
type ScalingErrorType string

const (
	// ScalingErrorTypeResourceExhausted indicates that the lifecycle manager could not create the instance due to resource exhaustion for an instance type in an availability zone.
	ScalingErrorTypeResourceExhausted ScalingErrorType = "ResourceExhaustedError"
	// ScalingErrorTypeCreationTimeout indicates that the lifecycle manager could not create the instance within its configured timeout despite multiple attempts.
	ScalingErrorTypeCreationTimeout ScalingErrorType = "CreationTimeoutError"
)

// ScaleOutErrorInfo is the backoff information for each instance type + zone.
type ScaleOutErrorInfo struct {
	// AvailabilityZone is the availability zone of the node pool.
	AvailabilityZone string `json:"availabilityZone"`
	// InstanceType is the instance type of the node pool.
	InstanceType string `json:"instanceType"`
	// ErrorType is the type of error that occurred during scale-out.
	ErrorType ScalingErrorType `json:"errorType"`
	// FailCount is the number of nodes that have failed creation.
	FailCount int32 `json:"failCount"`
}

// ScaleInErrorInfo is the information about nodes that could not be deleted for scale-in.
type ScaleInErrorInfo struct {
	// NodeNames is the list of node names that could not be deleted for scaled in.
	NodeNames []string `json:"nodeNames"`
}
