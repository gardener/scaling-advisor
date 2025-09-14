// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

// Service is a component that can be started and stopped.
type Service interface {
	// Start starts the service with the given context. Start may block depending on the implementation - if the service is a server.
	// The context is expected to be populated with a logger.
	Start(ctx context.Context) error

	// Stop stops the service. Stop does not block.
	Stop(ctx context.Context) error
}

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

// QPSBurst is a simple encapsulation of client QPS and Burst settings.
type QPSBurst struct {
	QPS   float32 `json:"qps"`
	Burst int     `json:"burst"`
}

// ConstraintReference is a reference to the ClusterScalingConstraint for which this advice is generated.
type ConstraintReference struct {
	// Name is the name of the ClusterScalingConstraint.
	Name string `json:"name"`
	// Namespace is the namespace of the ClusterScalingConstraint.
	Namespace string `json:"namespace"`
}

// NodeScoringStrategy represents a node scoring strategy variant.
type NodeScoringStrategy string

const (
	LeastWasteNodeScoringStrategy NodeScoringStrategy = "LeastWaste"
	LeastCostNodeScoringStrategy  NodeScoringStrategy = "LeastCost"
)

type CloudProvider string

const (
	AWSCloudProvider       CloudProvider = "aws"
	GCPCloudProvider       CloudProvider = "gcp"
	AzureCloudProvider     CloudProvider = "azure"
	AliCloudProvider       CloudProvider = "ali"
	OpenStackCloudProvider CloudProvider = "openstack"
)

func AsCloudProvider(cloudProvider string) (CloudProvider, error) {
	switch cloudProvider {
	case "aws":
		return AWSCloudProvider, nil
	case "gcp":
		return GCPCloudProvider, nil
	case "azure":
		return AzureCloudProvider, nil
	case "ali":
		return AliCloudProvider, nil
	case "openstack":
		return OpenStackCloudProvider, nil
	default:
		return "", fmt.Errorf("unuspported cloud provider: %s", cloudProvider)
	}
}

// ClientAccessMode indicates the access mode of k8s client
type ClientAccessMode string

const (
	ClientAccessNetwork  ClientAccessMode = "Network"
	ClientAccessInMemory ClientAccessMode = "InMemory"
)

// ClientFacades is a holder for the primary k8s client and informer interfaces
type ClientFacades struct {
	Mode               ClientAccessMode
	Client             kubernetes.Interface
	DynClient          dynamic.Interface
	InformerFactory    informers.SharedInformerFactory
	DynInformerFactory dynamicinformer.DynamicSharedInformerFactory
}
