// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// +k8s:deepcopy-gen=package
// +kubebuilder:object:generate=true
// +groupName=scad.gardener.cloud

// Package v1alpha1 defines the v1alpha1 core API for the scaling-advisor.
//
// The API models the inputs, outputs, and catalog data needed to generate
// cluster scaling advice. All resources are namespace-scoped for multi-tenant isolation.
//
// Primary resources:
//
//   - [ScalingConstraint]: operator input - declares the node pools, node template
//     references, availability zones, and planner/simulator strategy preferences that
//     govern how scaling advice is generated.
//
//   - [ScalingAdvice]: operator output - contains the generated [ScaleOutPlan] and
//     [ScaleInPlan] together with per-item diagnostics and overall execution feedback.
//
//   - [ScalingFeedback]: execution feedback written back by the operator after a
//     scaling action has been attempted, recording success/failure per scale item.
//
// Catalog resources (read by the operator, managed externally):
//
//   - [NodeTemplateSet]: defines the set of [NodeTemplate]s available for scale-out,
//     each describing an instance type with its resource capacity and reserved amounts.
//     [NodePool]s in a [ScalingConstraint] reference templates by name via [NodeTemplateRef].
//
//   - [InstanceTypeDefSet]: defines the base resource capacities for each instance type
//     used to populate simulated nodes during scale-out planning.
//
//   - [InstancePriceSet]: defines per-resource unit pricing for instance types, used by
//     cost-aware scoring strategies.
package v1alpha1
