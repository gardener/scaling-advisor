// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package api defines the public API for MinKAPI, a minimal Kubernetes API server
// used to front the kube-scheduler during scaling simulations.
//
// The central abstraction is [View], an in-memory snapshot of Kubernetes resources
// presented to the scheduler. A [View] is populated via [ResourceStore] (per-kind
// storage with add/update/delete/list) and [EventSink] (Kubernetes event recording).
//
// [Server] is the MinKAPI server that serves the kube-scheduler over the Kubernetes
// API. It is configured via [Config] and [WatchConfig] and assembled together with
// its dependencies by [App].
//
// [MatchCriteria] provides label-selector and field-selector filtering when querying
// objects from a [ResourceStore].
//
// Resource-kind metadata (GVK, GVR, list types, API discovery) is captured in
// [Descriptor], with the full set of supported kinds available via [SupportedScheme]
// and the [Descriptors] registry. [ClientFacades] supplies k8s client and informer
// access to the server at runtime.
package api
