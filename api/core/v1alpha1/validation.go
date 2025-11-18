package v1alpha1

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateNodePool validates a NodePool object.
func ValidateNodePool(np *NodePool, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(np.Region) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("region"), "region must not be empty"))
	}
	if len(np.AvailabilityZones) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("availabilityZones"), "availabilityZone must not be empty"))
	}
	if len(np.NodeTemplates) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("nodeTemplates"), "at least one nodeTemplate must be specified"))
	}
	if np.Priority < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("priority"), np.Priority, "priority must be non-negative"))
	}
	// TODO add checks for Quota
	return allErrs
}

// ValidateClusterScalingConstraint validates the given scaling constraints under the given fieldPath and returns a list of validation errors encapsulated in field.ErrorList
func ValidateClusterScalingConstraint(constraint *ClusterScalingConstraint, fieldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(constraint.Name) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("name"), "constraint name must not be empty"))
	}
	if strings.TrimSpace(constraint.Namespace) == "" {
		allErrs = append(allErrs, field.Required(fieldPath.Child("namespace"), "constraint namespace must not be empty"))
	}
	if !AllScalingAdviceGenerationModes.Has(constraint.Spec.AdviceGenerationMode) {
		allErrs = append(allErrs, field.Invalid(fieldPath.Child("spec.adviceGenerationMode"), constraint.Spec.AdviceGenerationMode, "invalid AdviceGenerationMode"))
	}
	return allErrs
}
