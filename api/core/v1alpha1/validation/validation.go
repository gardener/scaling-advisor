// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"

	"github.com/gardener/scaling-advisor/api/core/v1alpha1"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateScalingConstraint validates the given [v1alpha1.ScalingConstraint] under the given [field.Path], checking template references
// against the given [v1alpha1.NodeTemplateSet] and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScalingConstraint(constraint *v1alpha1.ScalingConstraint, templateSet *v1alpha1.NodeTemplateSet, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(constraint.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "constraint name must not be empty"))
	}
	if strings.TrimSpace(constraint.Namespace) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("namespace"), "constraint namespace must not be empty"))
	}
	allErrs = append(allErrs, ValidateScalingConstraintSpec(&constraint.Spec, &templateSet.Spec, fldPath.Child("spec"))...)
	return
}

// ValidateScalingConstraintSpec validates the given [v1alpha1.ScalingConstraintSpec], checking template references against
// the given [v1alpha1.NodeTemplateSetSpec] and returns a list of validation errors encapsulated in field.ErrorList.
func ValidateScalingConstraintSpec(constraintSpec *v1alpha1.ScalingConstraintSpec, templateSetSpec *v1alpha1.NodeTemplateSetSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	if len(constraintSpec.NodePools) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("nodePools"), "nodePools must not be empty"))
	}
	availableTemplateNames := sets.New[string]()
	for _, nt := range templateSetSpec.Templates {
		availableTemplateNames.Insert(nt.Name)
	}
	for i := range constraintSpec.NodePools {
		allErrs = append(allErrs, ValidateNodePool(&constraintSpec.NodePools[i], availableTemplateNames, fldPath.Child("nodePools").Index(i))...)
	}
	return
}

// ValidateNodeTemplateSet validates the given [v1alpha1.NodeTemplateSet] and returns a list of validation errors
// encapsulated in [field.ErrorList].
func ValidateNodeTemplateSet(templateSet *v1alpha1.NodeTemplateSet, fldPath *field.Path) (allErrs field.ErrorList) {
	return ValidateNodeTemplateSetSpec(&templateSet.Spec, fldPath.Child("spec"))
}

// ValidateNodeTemplateSetSpec validates the given [v1alpha1.NodeTemplateSetSpec] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateNodeTemplateSetSpec(templateSpec *v1alpha1.NodeTemplateSetSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	nodeTemplates := templateSpec.Templates
	if len(templateSpec.Templates) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("templates"), "must contain at least one NodeTemplate"))
	}
	names := sets.New[string]()
	for i, t := range nodeTemplates {
		allErrs = append(allErrs,
			ValidateNodeTemplate(
				&templateSpec.Templates[i],
				fldPath.Child("templates").Index(i),
			)...)
		if names.Has(t.Name) {
			allErrs = append(allErrs,
				field.Duplicate(
					fldPath.Child("templates").Index(i).Child("name"),
					t.Name,
				))
		}
		names.Insert(t.Name)
	}
	return
}

// ValidateInstancePriceSet validates the given [v1alpha1.InstancePriceSet] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateInstancePriceSet(priceSet *v1alpha1.InstancePriceSet, fldPath *field.Path) (allErrs field.ErrorList) {
	return ValidateInstancePriceSetSpec(&priceSet.Spec, fldPath.Child("spec"))
}

// ValidateInstancePriceSetSpec validates the given [v1alpha1.InstancePriceSetSpec] under the given [field.Path]
// and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateInstancePriceSetSpec(priceSetSpec *v1alpha1.InstancePriceSetSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	prices := priceSetSpec.Prices
	if len(prices) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("prices"), "must contain at least one InstancePrice"))
	}
	keys := sets.New[v1alpha1.InstancePriceKey]()
	for i, t := range prices {
		if keys.Has(t.InstancePriceKey) {
			allErrs = append(allErrs,
				field.Duplicate(
					fldPath.Child("prices").Index(i),
					t.InstancePriceKey,
				))
		}
		keys.Insert(t.InstancePriceKey)
	}
	return
}

// ValidateNodePool validates the given [v1alpha1.NodePool] object under the given [field.Path], checking referenced template names
// against set of available template names and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateNodePool(np *v1alpha1.NodePool, availableTemplateNames sets.Set[string], fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(np.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name must not be empty"))
	}
	if strings.TrimSpace(np.Region) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("region"), "region must not be empty"))
	}
	if len(np.AvailabilityZones) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("availabilityZones"), "availabilityZone must not be empty"))
	}
	if np.Priority < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("priority"), np.Priority, "priority must be non-negative"))
	}
	for i, ntr := range np.NodeTemplateRefs {
		if !availableTemplateNames.Has(ntr.Name) {
			allErrs = append(allErrs,
				field.NotFound(
					fldPath.Child("nodeTemplateRefs").Index(i).Child("name"),
					ntr.Name,
				))
		}
	}
	// TODO add checks for Quota
	return
}

// ValidateNodeTemplate validates the given [v1alpha1.NodeTemplate] object under the given [field.Path] and returns a
// list of validation errors encapsulated in [field.ErrorList].
func ValidateNodeTemplate(nt *v1alpha1.NodeTemplate, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(nt.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name must not be empty"))
	}
	if strings.TrimSpace(nt.Architecture) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("architecture"), "architecture must not be empty"))
	}
	if strings.TrimSpace(nt.InstanceType) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceType"), "instanceType must not be empty"))
	}
	return
}

// ValidateInstancePrice validates the given [v1alpha1.InstancePrice] object under the given [field.Path] and returns a
// list of validation errors encapsulated in [field.ErrorList].
func ValidateInstancePrice(p *v1alpha1.InstancePrice, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(p.InstanceType) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceType"), "instanceType must not be empty"))
	}
	if strings.TrimSpace(p.Region) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("region"), "region must not be empty"))
	}
	if strings.TrimSpace(string(p.CapacityType)) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("capacityType"), "capacityType must not be empty"))
	}
	if p.Price <= 0 {
		allErrs = append(allErrs,
			field.Invalid(fldPath.Child("price"), p.Price, "price must be greater than zero"))
	}
	if p.UnitCPUPrice != nil && *p.UnitCPUPrice <= 0 {
		allErrs = append(allErrs,
			field.Invalid(fldPath.Child("unitCPUPrice"), *p.UnitCPUPrice,
				"unitCPUPrice must be greater than zero"))
	}
	if p.UnitMemoryPrice != nil && *p.UnitMemoryPrice <= 0 {
		allErrs = append(allErrs,
			field.Invalid(fldPath.Child("unitMemoryPrice"), *p.UnitMemoryPrice,
				"unitMemoryPrice must be greater than zero"))
	}
	return
}
