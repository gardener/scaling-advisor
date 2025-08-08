// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	configv1apha1 "github.com/gardener/scaling-advisor/api/config/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
)

// ValidateScalingAdvisorConfiguration validates the ScalingAdvisorConfiguration.
func ValidateScalingAdvisorConfiguration(config *configv1apha1.ScalingAdvisorConfiguration) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateClientConnectionConfiguration(config.ClientConnection, field.NewPath("clientConnection"))...)
	allErrs = append(allErrs, validateLeaderElectionConfiguration(config.LeaderElection, field.NewPath("leaderElection"))...)
	// TODO add validation here.
	return allErrs
}

func validateClientConnectionConfiguration(config componentbaseconfigv1alpha1.ClientConnectionConfiguration, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if config.Burst < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("burst"), config.Burst, "burst must be non-negative"))
	}
	return allErrs
}

func validateLeaderElectionConfiguration(config componentbaseconfigv1alpha1.LeaderElectionConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if config.LeaderElect == nil || !*config.LeaderElect {
		return allErrs
	}
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(config.LeaseDuration, fldPath.Child("leaseDuration"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(config.RenewDeadline, fldPath.Child("renewDeadline"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(config.RetryPeriod, fldPath.Child("retryPeriod"))...)
	if config.LeaseDuration.Duration <= config.RenewDeadline.Duration {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("leaseDuration"), config.RenewDeadline, "LeaseDuration must be greater than RenewDeadline"))
	}
	if len(config.ResourceLock) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("resourceLock"), config.ResourceLock, "resourceLock is required"))
	}
	if len(config.ResourceNamespace) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("resourceNamespace"), config.ResourceNamespace, "resourceNamespace is required"))
	}
	if len(config.ResourceName) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("resourceName"), config.ResourceName, "resourceName is required"))
	}
	return allErrs
}

func mustBeGreaterThanZeroDuration(duration metav1.Duration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if duration.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, duration, "must be greater than 0"))
	}
	return allErrs
}
