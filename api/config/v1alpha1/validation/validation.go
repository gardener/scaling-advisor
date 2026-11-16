// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"

	configv1alpha1 "github.com/gardener/scaling-advisor/api/config/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateScalingAdvisorConfiguration validates the [configv1alpha1.OperatorConfig].
func ValidateScalingAdvisorConfiguration(config *configv1alpha1.OperatorConfig) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateServerConfiguration(config.Server, field.NewPath("server"))...)
	allErrs = append(allErrs, validateClientConnectionConfiguration(config.ClientConnection, field.NewPath("clientConnection"))...)
	allErrs = append(allErrs, validateLeaderElectionConfiguration(config.LeaderElection, field.NewPath("leaderElection"))...)
	allErrs = append(allErrs, validateControllersConfig(config.Controllers, field.NewPath("controllers"))...)
	return allErrs
}

// validateServerConfiguration validates the [configv1alpha1.ScalingAdvisorServerConfig].
func validateServerConfiguration(config configv1alpha1.ScalingAdvisorServerConfig, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if strings.TrimSpace(config.HealthProbeBindAddress) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("healthProbeBindAddress"), "healthProbeBindAddress must not be empty"))
	}
	if strings.TrimSpace(config.MetricsBindAddress) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("metricsBindAddress"), "metricsBindAddress must not be empty"))
	}
	if config.ProfilingEnabled && strings.TrimSpace(config.ProfilingBindAddress) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("profilingBindAddress"), "profilingBindAddress must not be empty when profilingEnabled is true"))
	}
	return allErrs
}

// validateClientConnectionConfiguration validates the [configv1alpha1.ClientConnectionConfig].
func validateClientConnectionConfiguration(config configv1alpha1.ClientConnectionConfig, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if config.QPS < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("qps"), config.QPS, "qps must be non-negative"))
	}
	if config.Burst < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("burst"), config.Burst, "burst must be non-negative"))
	}
	return allErrs
}

// validateLeaderElectionConfiguration validates the [configv1alpha1.LeaderElectionConfig].
func validateLeaderElectionConfiguration(config configv1alpha1.LeaderElectionConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !config.Enabled {
		return allErrs
	}
	leaseDurationErrs := mustBeGreaterThanZeroDuration(config.LeaseDuration, fldPath.Child("leaseDuration"))
	renewDeadlineErrs := mustBeGreaterThanZeroDuration(config.RenewDeadline, fldPath.Child("renewDeadline"))
	retryPeriodErrs := mustBeGreaterThanZeroDuration(config.RetryPeriod, fldPath.Child("retryPeriod"))
	allErrs = append(allErrs, leaseDurationErrs...)
	allErrs = append(allErrs, renewDeadlineErrs...)
	allErrs = append(allErrs, retryPeriodErrs...)
	if len(leaseDurationErrs) == 0 && len(renewDeadlineErrs) == 0 {
		if config.LeaseDuration.Duration <= config.RenewDeadline.Duration {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("leaseDuration"), config.LeaseDuration, "leaseDuration must be greater than renewDeadline"))
		}
	}
	if len(renewDeadlineErrs) == 0 && len(retryPeriodErrs) == 0 {
		if config.RetryPeriod.Duration >= config.RenewDeadline.Duration {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("retryPeriod"), config.RetryPeriod, "retryPeriod must be less than renewDeadline"))
		}
	}
	if strings.TrimSpace(config.ResourceLock) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("resourceLock"), "resourceLock is required"))
	}
	if strings.TrimSpace(config.ResourceNamespace) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("resourceNamespace"), "resourceNamespace is required"))
	}
	if strings.TrimSpace(config.ResourceName) == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("resourceName"), "resourceName is required"))
	}
	return allErrs
}

// validateControllersConfig validates the [configv1alpha1.ControllersConfig].
func validateControllersConfig(config configv1alpha1.ControllersConfig, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	scFldPath := fldPath.Child("scalingConstraints")
	if config.ScalingConstraints.ConcurrentSyncs <= 0 {
		allErrs = append(allErrs, field.Invalid(scFldPath.Child("concurrentSyncs"), config.ScalingConstraints.ConcurrentSyncs, "concurrentSyncs must be greater than 0"))
	}
	return allErrs
}

// mustBeGreaterThanZeroDuration validates that a duration is greater than zero.
func mustBeGreaterThanZeroDuration(duration metav1.Duration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if duration.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, duration, "must be greater than 0"))
	}
	return allErrs
}
