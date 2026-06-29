// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateNodePool validates a NodePool object.
func ValidateNodePool(np *NodePool, fieldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(np.Name) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("name"), "name must not be empty"))
	}
	if strings.TrimSpace(np.Region) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("region"), "region must not be empty"))
	}
	if len(np.AvailabilityZones) == 0 {
		allErrs = append(allErrs, field.Required(fieldPath.Child("availabilityZones"), "availabilityZone must not be empty"))
	}
	if np.Priority < 0 {
		allErrs = append(allErrs, field.Invalid(fieldPath.Child("priority"), np.Priority, "priority must be non-negative"))
	}
	if len(np.Requirements) == 0 {
		allErrs = append(allErrs, field.Required(fieldPath.Child("requirements"), "requirements must not be empty"))
	}
	// TODO add checks for Quota
	return allErrs
}

// ValidateNodeTemplate validates a NodeTemplate object.
//
//	type NodeTemplate struct {
//		// Capacity defines the capacity of resources that are available for this instance type.
//		Capacity corev1.ResourceList `json:"capacity"`
//		// KubeReserved defines the capacity for kube reserved resources.
//		// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#kube-reserved for additional information.
//		// +optional
//		KubeReserved corev1.ResourceList `json:"kubeReservedCapacity,omitempty"`
//		// SystemReserved defines the capacity for system reserved resources.
//		// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#system-reserved for additional information.
//		// Please read https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#general-guidelines when deciding to
//		// +optional
//		SystemReserved corev1.ResourceList `json:"systemReservedCapacity,omitempty"`
//		// Name is the name of the node template. Name is unique within a particular [ScalingConstraintSpec]
//		Name string `json:"name"`
//		// Architecture is the architecture of the instance type.
//		Architecture string `json:"architecture"`
//		// InstanceType is the instance type of the node template.
//		InstanceType string `json:"instanceType"`
//		// MaxVolumes is the max number of volumes that can be attached to a node of this instance type.
//		MaxVolumes int32 `json:"maxVolumes,omitzero"`
//	}
func ValidateNodeTemplate(nt *NodeTemplate, fieldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(nt.Name) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("name"), "name must not be empty"))
	}
	if strings.TrimSpace(nt.Architecture) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("architecture"), "architecture must not be empty"))
	}
	if strings.TrimSpace(nt.InstanceType) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("instanceType"), "instanceType must not be empty"))
	}
	return allErrs
}

// ValidateClusterScalingConstraint validates the given scaling constraints under the given fieldPath and returns a list of validation errors encapsulated in field.ErrorList
func ValidateClusterScalingConstraint(constraint *ScalingConstraint, fieldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(constraint.Name) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("name"), "constraint name must not be empty"))
	}
	if strings.TrimSpace(constraint.Namespace) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("namespace"), "constraint namespace must not be empty"))
	}
	allErrs = append(allErrs, ValidateClusterScalingConstraintSpec(&constraint.Spec, field.NewPath("spec"))...)
	return allErrs
}

func ValidateClusterScalingConstraintSpec(spec *ScalingConstraintSpec, fieldPath *field.Path) (allErrs field.ErrorList) {
	if len(spec.NodePools) == 0 {
		allErrs = append(allErrs, field.Required(fieldPath.Child("nodePools"), "nodePools must not be empty"))
	}
	if len(spec.NodeTemplates) == 0 {
		allErrs = append(allErrs, field.Required(fieldPath.Child("nodeTemplates"), "nodeTemplates must not be empty"))
	}

	//Validate each NodePool
	for i, np := range spec.NodePools {
		allErrs = append(allErrs, ValidateNodePool(&np, fieldPath.Child("nodePools").Index(i))...)
	}

	//Validate each NodeTemplate
	for i, nt := range spec.NodeTemplates {
		allErrs = append(allErrs, ValidateNodeTemplate(&nt, fieldPath.Child("nodeTemplates").Index(i))...)
	}

	return allErrs
}

func ValidateNodePoolRequirements(reqs []NodePoolRequirement, ntList []NodeTemplate, fieldPath *field.Path) (allErrs field.ErrorList) {
	for i, req := range reqs {
		matched := false
		for _, nt := range ntList {
			var val string
			switch req.Key {
			case corev1.LabelInstanceTypeStable:
				val = nt.InstanceType
			case corev1.LabelArchStable:
				val = nt.Architecture
			default:
				continue
			}

			switch req.Operator {
			case NodePoolRequirementOpIn:
				matched = slices.Contains(req.Values, val)
			case NodePoolRequirementOpNotIn:
				matched = !slices.Contains(req.Values, val)
			case NodePoolRequirementOpExists:
				matched = val != ""
			case NodePoolRequirementOpDoesNotExist:
				matched = val == ""
			}

			if matched {
				break
			}
		}
		if !matched {
			allErrs = append(allErrs, field.Invalid(fieldPath.Index(i), req, "requirement does not match any node template"))
		}
	}
	return allErrs
}
