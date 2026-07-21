// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// +k8s:deepcopy-gen=package
// +kubebuilder:object:generate=true
// +groupName=sa.gardener.cloud

// Package v1alpha1 defines the v1alpha1 API for the Scaling Advisor.
//
// The API models the inputs, outputs, and supporting catalog data required to
// generate cluster scaling advice.
//
// The primary resources are:
//
//   - ScalingConstraint, which defines the cluster-specific constraints and
//     preferences used when generating scaling advice.
//   - ScalingAdvice, which contains the generated scale-out and scale-in
//     recommendations together with execution feedback and diagnostics.
//
// Supporting catalog resources are:
//
//   - NodeTemplateSet, which defines the set of node templates available to the
//     scaling advisor.
//   - InstancePriceSet, which defines pricing information for the instance
//     types referenced by NodeTemplate objects.
//
// The API follows Kubernetes conventions by exposing configuration through Spec
// and controller-generated information through Status where appropriate.
package v1alpha1
