package v1alpha1

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateNodePool validates a NodePool object.
func ValidateNodePool(np *NodePool, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
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
