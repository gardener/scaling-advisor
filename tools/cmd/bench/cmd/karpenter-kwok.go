// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/sets"
)

// * FIXME! Src: kwok/cloudprovider/helpers.go
type InstanceTypeOptions struct {
	Name             string              `json:"name"`
	Offerings        KWOKOfferings       `json:"offerings"`
	Architecture     string              `json:"architecture"`
	OperatingSystems []corev1.OSName     `json:"operatingSystems"`
	Resources        corev1.ResourceList `json:"resources"`

	// These are used for setting default requirements, they should not be used
	// for setting arbitrary node labels.  Set the labels on the created NodePool for
	// that use case.
	instanceTypeLabels map[string]string
}

type KWOKOfferings []KWOKOffering

type KWOKOffering struct {
	Offering
	Requirements []corev1.NodeSelectorRequirement
}

// An Offering describes where an InstanceType is available to be used, with the expectation that its properties
// may be tightly coupled (e.g. the availability of an instance type in some zone is scoped to a capacity type) and
// these properties are captured with labels in Requirements.
// Requirements are required to contain the keys v1.CapacityTypeLabelKey and corev1.LabelTopologyZone.
type Offering struct {
	Requirements
	Price               float64
	Available           bool
	ReservationCapacity int
}

// Requirements are an efficient set representation under the hood. Since its underlying
// types are slices and maps, this type should not be used as a pointer.
type Requirements map[string]*Requirement

type Requirement struct {
	Key         string
	complement  bool
	values      sets.Set[string]
	greaterThan *int
	lessThan    *int
	MinValues   *int
}
