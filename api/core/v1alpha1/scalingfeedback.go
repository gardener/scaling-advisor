// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apicommon "github.com/gardener/scaling-advisor/api/common/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScalingFeedback provides scale-in and scale-out feedback from the lifecycle manager.
// Scaling advisor can refine its future scaling advice based on this feedback.
type ScalingFeedback struct {
	// ConstraintRef is a reference to the ScalingConstraint that this advice is based on.
	ConstraintRef apicommon.NamespacedName `json:"constraintRef"`
	// ScaleOut is the scale-out feedback from the lifecycle manager when applying [ScaleOutPlan]
	// [ScalingAdviceSpec].
	ScaleOut *ScaleOutFeedback `json:"scaleOut,omitempty"`
	// ScaleIn is the scale-in feedback from life-cycle manager when applying [ScaleInPlan]
	ScaleIn *ScaleInFeedback `json:"scaleIn,omitempty"`
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

// ScaleOutFeedback is the feedback from the life cycle manager when applying an [ScaleOutPlan] of a [ScalingAdviceSpec]
type ScaleOutFeedback struct {
	Items []ScaleOutItemFeedback `json:"items,omitempty"`
}

// ScaleOutItemFeedback is the feedback from the life cycle manager when applying an individual [ScaleOutItem]
type ScaleOutItemFeedback struct {
	// Index represents the item index in [ScaleOutPlan.Items]
	// +required
	Index int32 `json:"index"`
	// CreationDeadline represents the time after which the scaling-advisor can expect real nodes to be created and available
	// for the corresponding [ScaleOutItem]'s [NodePlacement]. When the [ScalingFeedback] is constructed by the life-cycle manager,
	// this field is mandatory to be set inside all [ScaleOutItemFeedback]
	// +required
	CreationDeadline metav1.Time `json:"creationDeadline"`
	// BackoffUtil if populated, represents the time until the scaling-advisor will not consider the corresponding
	// [ScaleOutItem]'s [NodePlacement] when running simulations and generating subsequent [ScaleOutPlan]'s
	// +optional
	BackoffUntil *metav1.Time `json:"backoffUntil,omitempty"`
	// ErrorType is the type of error that occurred during scale-out.
	// +optional
	ErrorType ScalingErrorType `json:"errorType,omitempty"`
	// FailCount is the number of nodes that have failed creation.
	// +optional
	FailCount int32 `json:"failCount,omitzero"`
}

// ScaleInFeedback is  the feedback from the life cycle manager after applying [ScaleInPlan]
type ScaleInFeedback struct {
	// AcceptedNodeNames holds the slice of node names that were accepted for scale-in by the lifecycle controller.
	// Required to be specified, since if empty, the ScaleInFeedback itself should not be populated.
	// +required
	AcceptedNodesNames []string `json:"acceptedNodesNames"`
}
