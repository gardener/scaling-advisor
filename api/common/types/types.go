// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerConfig is the configuration for services that are servers.
type ServerConfig struct {
	HostPort `json:",inline"`
	// KubeConfigPath is the path to master kube-config.
	KubeConfigPath string `json:"kubeConfigPath"`
	// ProfilingEnable indicates whether this service should register the standard pprof HTTP handlers: /debug/pprof/*
	ProfilingEnabled bool `json:"profilingEnabled"`
	// GracefulShutdownTimeout is the time given to the service to gracefully shutdown.
	GracefulShutdownTimeout metav1.Duration `json:"gracefulShutdownTimeout"`
}

// HostPort contains information for service host and port.
type HostPort struct {
	// Host is the IP address on which to listen for the specified port.
	Host string `json:"host"`
	// Port is the port on which to serve requests.
	Port int `json:"port"`
}

// ConstraintReference is a reference to the ClusterScalingConstraint for which this advice is generated.
type ConstraintReference struct {
	// Name is the name of the ClusterScalingConstraint.
	Name string `json:"name"`
	// Namespace is the namespace of the ClusterScalingConstraint.
	Namespace string `json:"namespace"`
}
