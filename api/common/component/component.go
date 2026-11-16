// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package component contains operational types shared across scaling-advisor server components:
// server configuration, lifecycle interfaces, and client rate-limit settings.
// Pure API enums and value types live in the parent [common] package.
package component

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// QPSBurst is a simple encapsulation of client QPS and Burst settings.
type QPSBurst struct {
	// QPS is the queries per second rate limit for the client.
	QPS float32 `json:"qps"`
	// Burst is the burst size for rate limiting, allowing temporary spikes above QPS.
	Burst int `json:"burst"`
}

// ServerConfig is the common configuration for a server which can be used as standalone
// or embedded within another process.
type ServerConfig struct {
	// KubeConfigPath is the path to master kube-config.
	KubeConfigPath string `json:"kubeConfigPath"`
	// BindAddress is the address(host:port) to bind the server to.
	BindAddress string `json:"bindAddress"`
	// GracefulShutdownTimeout is the time given to the core to gracefully shutdown.
	GracefulShutdownTimeout metav1.Duration `json:"gracefulShutdownTimeout"`
	// ProfilingEnabled indicates whether this core should register the standard pprof HTTP handlers: /debug/pprof/*
	ProfilingEnabled bool `json:"profilingEnabled"`
}

// Service is a component that can be started and stopped.
type Service interface {
	// Start starts the core with the given context. Start may block depending on the implementation - if the core is a server.
	// The context is expected to be populated with a logger.
	Start(ctx context.Context) error
	// Stop stops the core. Stop does not block.
	Stop(ctx context.Context) error
}

// Resettable defines types that can reset their state to a default or initial configuration.
type Resettable interface {
	// Reset resets the state of the implementing type.
	Reset() error
}
