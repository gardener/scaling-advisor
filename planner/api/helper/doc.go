// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package helper provides utility and adapter functions that operate on the
// planner and simulation API types alongside standard Kubernetes types.
//
// It follows the convention established by [k8s.io/kubernetes/pkg/apis/core/v1/helper]:
// helper functions that are too dependency-heavy for the API package itself
// (because they import sibling API modules) are placed in a sub-package so
// that lightweight consumers such as [github.com/gardener/scaling-advisor/minkapi]
// can import [github.com/gardener/scaling-advisor/planner/api] without
// transitively pulling in [github.com/gardener/scaling-advisor/simulation/api]
// or [github.com/gardener/scaling-advisor/common].
//
// [k8s.io/kubernetes/pkg/apis/core/v1/helper]: https://pkg.go.dev/k8s.io/kubernetes/pkg/apis/core/v1/helper
package helper
