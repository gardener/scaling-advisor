// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/gardener/scaling-advisor/api/common/constants"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// GroupVersion is the version of the group for all scaling recommender custom resources.
	GroupVersion = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register objects from the scaling recommender API.
	SchemeGroupVersion = schema.GroupVersion{Group: constants.OperatorGroupName, Version: GroupVersion}
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ScalingAdvice{},
		&ScalingAdviceList{},
		&ScalingConstraint{},
		&ScalingConstraintList{},
		&ScalingFeedback{},
	)
	return nil
}
