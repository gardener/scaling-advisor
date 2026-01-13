// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/planner"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateScalingAdviceResponse creates a ScalingAdviceResponse based on the given request and plan result.
func CreateScalingAdviceResponse(request planner.ScalingAdviceRequest, planResult planner.ScalingPlanResult) *planner.ScalingAdviceResponse {
	scalingAdvice := sacorev1alpha1.ClusterScalingAdvice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      planResult.Name,
			Namespace: request.Constraint.Namespace,
			Labels:    planResult.Labels, // TODO use merge when there are additional labels to add other than from planResult
		},
		Spec: sacorev1alpha1.ClusterScalingAdviceSpec{
			ScaleOutPlan: planResult.ScaleOutPlan,
			ScaleInPlan:  planResult.ScaleInPlan,
			ConstraintRef: commontypes.ConstraintReference{
				Name:      request.Constraint.Name,
				Namespace: request.Constraint.Namespace,
			},
		},
	}
	scalingAdviceResponse := &planner.ScalingAdviceResponse{
		ScalingAdvice: &scalingAdvice,
		Diagnostics:   planResult.Diagnostics,
	}
	return scalingAdviceResponse
}
