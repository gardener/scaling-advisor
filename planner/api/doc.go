// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package api defines the public API for the scaling-advisor planner service.
//
// The primary entry point is [ScalingPlanner], which accepts a [Request] and delivers
// one or more [Response] values on a channel. A [Request] carries the [ClusterSnapshot],
// [sacorev1alpha1.ScalingConstraintSpec], and optional [Memento] from a prior invocation.
// Responses contain [sacorev1alpha1.ScaleOutPlan] and/or [sacorev1alpha1.ScaleInPlan] items.
package api
