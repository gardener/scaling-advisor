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

// ValidateClusterScalingConstraintSpec validates the given scaling constraint spec
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

// ValidateNodePoolRequirements validates node pool requirements by ensuring each requirement points to a node template
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
