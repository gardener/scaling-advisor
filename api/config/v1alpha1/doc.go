// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// +k8s:deepcopy-gen=package
// +k8s:defaulter-gen=TypeMeta
// +groupName=config.scad.gardener.cloud

// Package v1alpha1 defines the v1alpha1 configuration API for the scaling-advisor operator.
//
// The central type is [OperatorConfig], which covers:
//   - [ScalingAdvisorServerConfig]: bind addresses for health, metrics, and profiling endpoints.
//   - [ClientConnectionConfig]: kubeconfig path and client-side rate-limiting (QPS/Burst).
//   - [LeaderElectionConfig]: lease-based leader election parameters.
//   - [ControllersConfig]: per-controller concurrency settings.
//
// Defaults for a zero-value [OperatorConfig] are applied by [SetDefaults_OperatorConfig].
// The config is validated by [validation.ValidateScalingAdvisorConfiguration].
package v1alpha1
