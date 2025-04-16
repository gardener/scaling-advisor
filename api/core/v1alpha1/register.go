// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// GroupName is the name of the group for all scaling recommender custom resources.
	GroupName = "sr.gardener.cloud"
	// GroupVersion is the version of the group for all scaling recommender custom resources.
	GroupVersion = "v1alpha1"
)

var (
	// SchemeGroupVersion is group version used to register objects from the scaling recommender API.
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: GroupVersion}
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder
	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	localSchemeBuilder.Register(addKnownTypes)
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ClusterScalingConstraint{},
		&ClusterScalingConstraintList{},
	)
	return nil
}
