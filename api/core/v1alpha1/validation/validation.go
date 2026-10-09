// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"regexp"
	"strings"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	"github.com/gardener/scaling-advisor/api/core/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateScalingConstraint validates the given [v1alpha1.ScalingConstraint] under the given [field.Path], checking template references
// against the given [v1alpha1.NodeTemplateSet] and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScalingConstraint(constraint *v1alpha1.ScalingConstraint, templateSet *v1alpha1.NodeTemplateSet, fldPath *field.Path) (allErrs field.ErrorList) {
	metaPath := fldPath.Child("metadata")
	if strings.TrimSpace(constraint.Name) == "" {
		allErrs = append(allErrs, field.Required(metaPath.Child("name"), "constraint name must not be empty"))
	}
	if strings.TrimSpace(constraint.Namespace) == "" {
		allErrs = append(allErrs, field.Required(metaPath.Child("namespace"), "constraint namespace must not be empty"))
	}
	allErrs = append(allErrs, ValidateScalingConstraintSpec(&constraint.Spec, &templateSet.Spec, fldPath.Child("spec"))...)
	return
}

// ValidateScalingConstraintSpec validates the given [v1alpha1.ScalingConstraintSpec], checking template references against
// the given [v1alpha1.NodeTemplateSetSpec] and returns a list of validation errors encapsulated in field.ErrorList.
func ValidateScalingConstraintSpec(constraintSpec *v1alpha1.ScalingConstraintSpec, templateSetSpec *v1alpha1.NodeTemplateSetSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(string(constraintSpec.PlannerStrategy)) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("plannerStrategy"), "plannerStrategy must not be empty"))
	} else if !apicommon.SupportedPlannerStrategies.Has(constraintSpec.PlannerStrategy) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("plannerStrategy"), constraintSpec.PlannerStrategy, sets.List(apicommon.SupportedPlannerStrategies)))
	}
	if constraintSpec.SimulatorStrategy != "" && !apicommon.SupportedSimulatorStrategies.Has(constraintSpec.SimulatorStrategy) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("simulatorStrategy"), constraintSpec.SimulatorStrategy, sets.List(apicommon.SupportedSimulatorStrategies)))
	}
	if constraintSpec.ScoringStrategy != "" && !apicommon.SupportedNodeScoringStrategies.Has(constraintSpec.ScoringStrategy) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("scoringStrategy"), constraintSpec.ScoringStrategy, sets.List(apicommon.SupportedNodeScoringStrategies)))
	}
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

// ValidateNodeTemplateSetAgainstInstanceTypeDefSet validates that every [v1alpha1.NodeTemplate.InstanceType]
// in the given [v1alpha1.NodeTemplateSet] resolves to a known [v1alpha1.InstanceTypeDef] in the given
// [v1alpha1.InstanceTypeDefSet], and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateNodeTemplateSetAgainstInstanceTypeDefSet(templateSet *v1alpha1.NodeTemplateSet, defSet *v1alpha1.InstanceTypeDefSet, fldPath *field.Path) (allErrs field.ErrorList) {
	return ValidateNodeTemplateSetSpecAgainstInstanceTypeDefSet(&templateSet.Spec, &defSet.Spec, fldPath.Child("spec"))
}

// ValidateNodeTemplateSetSpecAgainstInstanceTypeDefSet validates that every [v1alpha1.NodeTemplate.InstanceType]
// in the given [v1alpha1.NodeTemplateSetSpec] resolves to a known [v1alpha1.InstanceTypeDef] in the given
// [v1alpha1.InstanceTypeDefSetSpec], and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateNodeTemplateSetSpecAgainstInstanceTypeDefSet(templateSetSpec *v1alpha1.NodeTemplateSetSpec, defSetSpec *v1alpha1.InstanceTypeDefSetSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	knownInstanceTypes := sets.New[string]()
	for _, d := range defSetSpec.InstanceTypeDefs {
		knownInstanceTypes.Insert(d.Name)
	}
	for i, t := range templateSetSpec.Templates {
		if !knownInstanceTypes.Has(t.InstanceType) {
			allErrs = append(allErrs, field.NotFound(
				fldPath.Child("templates").Index(i).Child("instanceType"),
				t.InstanceType,
			))
		}
	}
	return
}

// ValidateInstancePriceSetAgainstInstanceTypeDefSet validates that every [v1alpha1.InstancePriceKey.InstanceType]
// in the given [v1alpha1.InstancePriceSet] resolves to a known [v1alpha1.InstanceTypeDef] in the given
// [v1alpha1.InstanceTypeDefSet], and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateInstancePriceSetAgainstInstanceTypeDefSet(priceSet *v1alpha1.InstancePriceSet, defSet *v1alpha1.InstanceTypeDefSet, fldPath *field.Path) (allErrs field.ErrorList) {
	return ValidateInstancePriceSetSpecAgainstInstanceTypeDefSet(&priceSet.Spec, &defSet.Spec, fldPath.Child("spec"))
}

// ValidateInstancePriceSetSpecAgainstInstanceTypeDefSet validates that every [v1alpha1.InstancePriceKey.InstanceType]
// in the given [v1alpha1.InstancePriceSetSpec] resolves to a known [v1alpha1.InstanceTypeDef] in the given
// [v1alpha1.InstanceTypeDefSetSpec], and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateInstancePriceSetSpecAgainstInstanceTypeDefSet(priceSetSpec *v1alpha1.InstancePriceSetSpec, defSetSpec *v1alpha1.InstanceTypeDefSetSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	knownInstanceTypes := sets.New[string]()
	for _, d := range defSetSpec.InstanceTypeDefs {
		knownInstanceTypes.Insert(d.Name)
	}
	for i, p := range priceSetSpec.Prices {
		if !knownInstanceTypes.Has(p.InstanceType) {
			allErrs = append(allErrs, field.NotFound(
				fldPath.Child("prices").Index(i).Child("instanceType"),
				p.InstanceType))
		}
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
			ValidateNodeTemplate(&templateSpec.Templates[i], fldPath.Child("templates").Index(i))...)
		if names.Has(t.Name) {
			allErrs = append(allErrs,
				field.Duplicate(fldPath.Child("templates").Index(i).Child("name"), t.Name))
		}
		names.Insert(t.Name)
	}
	return
}

// ValidateInstanceTypeDefSet validates the given [v1alpha1.InstanceTypeDefSet] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateInstanceTypeDefSet(defSet *v1alpha1.InstanceTypeDefSet, fldPath *field.Path) (allErrs field.ErrorList) {
	return ValidateInstanceTypeDefSetSpec(&defSet.Spec, fldPath.Child("spec"))
}

// ValidateInstanceTypeDefSetSpec validates the given [v1alpha1.InstanceTypeDefSetSpec] under the given [field.Path]
// and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateInstanceTypeDefSetSpec(spec *v1alpha1.InstanceTypeDefSetSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(string(spec.Provider)) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("provider"), "provider must not be empty"))
	}
	if len(spec.InstanceTypeDefs) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceTypeDefs"), "must contain at least one InstanceTypeDef"))
	}
	names := sets.New[string]()
	for i, d := range spec.InstanceTypeDefs {
		allErrs = append(allErrs, ValidateInstanceTypeDef(&spec.InstanceTypeDefs[i], fldPath.Child("instanceTypeDefs").Index(i))...)
		if names.Has(d.Name) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Child("instanceTypeDefs").Index(i).Child("name"), d.Name))
		} else {
			names.Insert(d.Name)
		}
	}
	return
}

// ValidateInstanceTypeDef validates the given [v1alpha1.InstanceTypeDef] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateInstanceTypeDef(d *v1alpha1.InstanceTypeDef, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(d.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name must not be empty"))
	}
	if strings.TrimSpace(d.Architecture) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("architecture"), "architecture must not be empty"))
	}
	if len(d.BaseResources) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("baseResources"), "baseResources must not be empty"))
	}
	if d.MaxVolumes < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxVolumes"), d.MaxVolumes, "maxVolumes must be non-negative"))
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
	if strings.TrimSpace(string(priceSetSpec.Provider)) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("provider"), "provider must not be empty"))
	}
	prices := priceSetSpec.Prices
	if len(prices) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("prices"), "must contain at least one InstancePrice"))
	}
	keys := sets.New[v1alpha1.InstancePriceKey]()
	for i, t := range prices {
		pricePath := fldPath.Child("prices").Index(i)
		allErrs = append(allErrs, ValidateInstancePrice(&priceSetSpec.Prices[i], pricePath)...)
		if keys.Has(t.InstancePriceKey) {
			allErrs = append(allErrs, field.Duplicate(pricePath, t.InstancePriceKey))
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
	if len(np.NodeTemplateRefs) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("nodeTemplateRefs"), "must contain at least one NodeTemplateRef"))
	}
	seenTemplateNames := sets.New[string]()
	for i, ntr := range np.NodeTemplateRefs {
		ntrPath := fldPath.Child("nodeTemplateRefs").Index(i)
		if seenTemplateNames.Has(ntr.Name) {
			allErrs = append(allErrs, field.Duplicate(ntrPath.Child("name"), ntr.Name))
		} else {
			seenTemplateNames.Insert(ntr.Name)
		}
		if !availableTemplateNames.Has(ntr.Name) {
			allErrs = append(allErrs, field.NotFound(ntrPath.Child("name"), ntr.Name))
		}
		allErrs = append(allErrs, ValidateNodeTemplateRef(&np.NodeTemplateRefs[i], ntrPath)...)
	}
	if np.ScalingParams != nil {
		allErrs = append(allErrs, ValidateScalingParams(np.ScalingParams, fldPath.Child("scalingParams"))...)
	}
	return
}

// ValidateScalingParams validates the given [v1alpha1.ScalingParams] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScalingParams(sp *v1alpha1.ScalingParams, fldPath *field.Path) (allErrs field.ErrorList) {
	seenNames := sets.New[string]()
	for i, ut := range sp.UtilizationThresholds {
		utPath := fldPath.Child("utilizationThresholds").Index(i)
		name := strings.TrimSpace(ut.Name.String())
		if name == "" {
			allErrs = append(allErrs, field.Required(utPath.Child("name"), "name must not be empty"))
		} else if seenNames.Has(name) {
			allErrs = append(allErrs, field.Duplicate(utPath.Child("name"), ut.Name))
		} else {
			seenNames.Insert(name)
		}
		if strings.TrimSpace(ut.Threshold) == "" {
			allErrs = append(allErrs, field.Required(utPath.Child("threshold"), "threshold must not be empty"))
		} else if !utilizationThresholdRe.MatchString(ut.Threshold) || ut.Threshold == "0" {
			allErrs = append(allErrs, field.Invalid(utPath.Child("threshold"), ut.Threshold, "threshold must be a decimal string in (0, 1] (e.g. \"0.5\" or \"1\")"))
		}
	}
	return
}

// ValidateNodeTemplateRef validates the given [v1alpha1.NodeTemplateRef] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateNodeTemplateRef(ntr *v1alpha1.NodeTemplateRef, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(ntr.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name must not be empty"))
	}
	if ntr.Quota != nil {
		allErrs = append(allErrs, ValidateResourceLimits(ntr.Quota, fldPath.Child("quota"))...)
	}
	return
}

// ValidateResourceLimits validates the given [v1alpha1.ResourceLimits] under the given [field.Path], checking
// that Min does not exceed Max for any resource, and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateResourceLimits(rl *v1alpha1.ResourceLimits, fldPath *field.Path) (allErrs field.ErrorList) {
	for resourceName, minQty := range rl.Min {
		maxQty, hasMax := rl.Max[resourceName]
		if hasMax && minQty.Cmp(maxQty) > 0 {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("min").Key(string(resourceName)),
				minQty.String(),
				"min must not exceed max for resource "+string(resourceName),
			))
		}
	}
	return
}

// ValidateScalingAdvice validates the given [v1alpha1.ScalingAdvice] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScalingAdvice(advice *v1alpha1.ScalingAdvice, fldPath *field.Path) (allErrs field.ErrorList) {
	return ValidateScalingAdviceSpec(&advice.Spec, fldPath.Child("spec"))
}

// ValidateScalingAdviceSpec validates the given [v1alpha1.ScalingAdviceSpec] under the given [field.Path]
// and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScalingAdviceSpec(spec *v1alpha1.ScalingAdviceSpec, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(spec.ConstraintRef.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("constraintRef", "name"), "constraintRef.name must not be empty"))
	}
	if strings.TrimSpace(spec.ConstraintRef.Namespace) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("constraintRef", "namespace"), "constraintRef.namespace must not be empty"))
	}
	if spec.ScaleOut != nil {
		allErrs = append(allErrs, ValidateScaleOutPlan(spec.ScaleOut, fldPath.Child("scaleOut"))...)
	}
	if spec.ScaleIn != nil {
		allErrs = append(allErrs, ValidateScaleInPlan(spec.ScaleIn, fldPath.Child("scaleIn"))...)
	}
	if spec.Diagnostic != nil {
		allErrs = append(allErrs, ValidateScalingAdviceDiagnostic(spec.Diagnostic, fldPath.Child("diagnostic"))...)
	}
	return
}

// ValidateScaleOutPlan validates the given [v1alpha1.ScaleOutPlan] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScaleOutPlan(plan *v1alpha1.ScaleOutPlan, fldPath *field.Path) (allErrs field.ErrorList) {
	if len(plan.Items) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("items"), "must contain at least one ScaleOutItem"))
	}
	for i := range plan.Items {
		allErrs = append(allErrs, ValidateScaleOutItem(&plan.Items[i], fldPath.Child("items").Index(i))...)
	}
	return
}

// ValidateScaleInPlan validates the given [v1alpha1.ScaleInPlan] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScaleInPlan(plan *v1alpha1.ScaleInPlan, fldPath *field.Path) (allErrs field.ErrorList) {
	if len(plan.Items) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("items"), "must contain at least one ScaleInItem"))
	}
	for i := range plan.Items {
		allErrs = append(allErrs, ValidateScaleInItem(&plan.Items[i], fldPath.Child("items").Index(i))...)
	}
	return
}

// ValidateScaleOutItem validates the given [v1alpha1.ScaleOutItem] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScaleOutItem(item *v1alpha1.ScaleOutItem, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(item.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name must not be empty"))
	}
	if item.CurrentReplicas < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("currentReplicas"), item.CurrentReplicas, "currentReplicas must be non-negative"))
	}
	if item.Delta < 1 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("delta"), item.Delta, "delta must be at least 1"))
	}
	allErrs = append(allErrs, ValidateNodePlacement(&item.NodePlacement, fldPath.Child("nodePlacement"))...)
	return
}

// ValidateScaleInItem validates the given [v1alpha1.ScaleInItem] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScaleInItem(item *v1alpha1.ScaleInItem, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(item.NodeName) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("nodeName"), "nodeName must not be empty"))
	}
	allErrs = append(allErrs, ValidateNodePlacement(&item.NodePlacement, fldPath.Child("nodePlacement"))...)
	return
}

// ValidateNodePlacement validates the given [v1alpha1.NodePlacement] under the given [field.Path] and
// returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateNodePlacement(np *v1alpha1.NodePlacement, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(np.PoolName) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("poolName"), "poolName must not be empty"))
	}
	if strings.TrimSpace(np.TemplateName) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("templateName"), "templateName must not be empty"))
	}
	if strings.TrimSpace(np.InstanceType) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceType"), "instanceType must not be empty"))
	}
	if strings.TrimSpace(np.Region) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("region"), "region must not be empty"))
	}
	if strings.TrimSpace(np.AvailabilityZone) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("availabilityZone"), "availabilityZone must not be empty"))
	}
	return
}

// ValidateScalingAdviceDiagnostic validates the given [v1alpha1.ScalingAdviceDiagnostic] under the given [field.Path]
// and returns a list of validation errors encapsulated in [field.ErrorList].
func ValidateScalingAdviceDiagnostic(d *v1alpha1.ScalingAdviceDiagnostic, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(d.TraceLogName) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("traceLogName"), "traceLogName must not be empty"))
	}
	if strings.TrimSpace(d.RunResultsLogName) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("runResultsLogName"), "runResultsLogName must not be empty"))
	}
	return
}

// ValidateNodeTemplate validates the given [v1alpha1.NodeTemplate] object under the given [field.Path] and returns a
// list of validation errors encapsulated in [field.ErrorList].
func ValidateNodeTemplate(nt *v1alpha1.NodeTemplate, fldPath *field.Path) (allErrs field.ErrorList) {
	if strings.TrimSpace(nt.Name) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name must not be empty"))
	}
	if strings.TrimSpace(nt.InstanceType) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("instanceType"), "instanceType must not be empty"))
	}
	if len(nt.ReservedResources) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("reservedResources"), "reservedResources must not be empty"))
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
	if strings.TrimSpace(p.Price) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("price"), "price must not be empty"))
	} else if !decimalPriceRe.MatchString(p.Price) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("price"), p.Price, "price must be a positive decimal string (e.g. \"0.192\")"))
	}
	seenResourceNames := sets.New[corev1.ResourceName]()
	for i, rup := range p.ResourceUnitPrice {
		rupPath := fldPath.Child("resourceUnitPrice").Index(i)
		if strings.TrimSpace(rup.Name.String()) == "" {
			allErrs = append(allErrs, field.Required(rupPath.Child("name"), "name must not be empty"))
		} else if seenResourceNames.Has(rup.Name) {
			allErrs = append(allErrs, field.Duplicate(rupPath.Child("name"), rup.Name))
		} else {
			seenResourceNames.Insert(rup.Name)
		}
		if strings.TrimSpace(rup.Value) == "" {
			allErrs = append(allErrs, field.Required(rupPath.Child("value"), "value must not be empty"))
		} else if !decimalPriceRe.MatchString(rup.Value) {
			allErrs = append(allErrs, field.Invalid(rupPath.Child("value"), rup.Value, "value must be a positive decimal string (e.g. \"0.048\")"))
		}
	}
	return
}

// decimalPriceRe matches the same pattern enforced by the kubebuilder +Pattern marker on
// InstancePrice.Price and ResourceUnitPrice.Value: a positive decimal string with no
// scientific notation, no sign, and no leading zeros (except "0.NNN" fractions).
var decimalPriceRe = regexp.MustCompile(`^(0\.[0-9]*[1-9]|[1-9][0-9]*(\.[0-9]+)?)$`)

// utilizationThresholdRe matches the kubebuilder +Pattern marker on ResourceUtilizationThreshold.Threshold.
// Bare "0" passes this pattern but is excluded by an explicit zero check in ValidateScalingParams.
var utilizationThresholdRe = regexp.MustCompile(`^(0(\.[0-9]+)?|1(\.0+)?)$`)
