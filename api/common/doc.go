// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package common contains unversioned shared API types, enums, constants, and sentinel errors
// used across scaling-advisor modules.
//
// The package is organized into three concerns:
//
//   - Types and enums: [NamespacedName], [CloudProvider], [PlannerStrategy], [SimulatorStrategy],
//     [NodeScoringStrategy], [ScalingAdviceGenerationMode], [CapacityType], and [ContextKey].
//
//   - Constants: operator and CRD group names, default bind addresses and ports, annotation and
//     label keys used across the system (see constants.go).
//
//   - Sentinel errors: package-level error values such as [ErrLoadTemplate],
//     [ErrLoadTemplate], and [ErrSerialization] for use with [errors.Is] (see errors.go).
package common
