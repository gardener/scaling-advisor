// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"cmp"
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// Resettable defines types that can reset their state to a default or initial configuration.
type Resettable interface {
	// Reset resets the state of the implementing type.
	Reset() error
}

// Service is a component that can be started and stopped.
type Service interface {
	// Start starts the service with the given context. Start may block depending on the implementation - if the service is a server.
	// The context is expected to be populated with a logger.
	Start(ctx context.Context) error
	// Stop stops the core. Stop does not block.
	Stop(ctx context.Context) error
}

// ServerConfig is the common configuration for a server.
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

// QPSBurst is a simple encapsulation of client QPS and Burst settings.
type QPSBurst struct {
	// QPS is the queries per second rate limit for the client.
	QPS float32 `json:"qps"`
	// Burst is the burst size for rate limiting, allowing temporary spikes above QPS.
	Burst int `json:"burst"`
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
}

// NodeScoringStrategy represents a node scoring strategy variant.
type NodeScoringStrategy string

const (
	// NodeScoringStrategyLeastWaste represents a scoring strategy that minimizes resource waste.
	NodeScoringStrategyLeastWaste NodeScoringStrategy = "least-waste"
	// NodeScoringStrategyLeastCost represents a scoring strategy that minimizes cost.
	NodeScoringStrategyLeastCost NodeScoringStrategy = "least-cost"
)

// CloudProvider represents the cloud provider type for the cluster.
// +enum
type CloudProvider string

const (
	// CloudProviderAWS indicates AWS as cloud provider.
	CloudProviderAWS CloudProvider = "aws"
	// CloudProviderGCP indicates GCP as cloud provider.
	CloudProviderGCP CloudProvider = "gcp"
	// CloudProviderAzure indicates Azure as cloud provider.
	CloudProviderAzure CloudProvider = "azure"
	// CloudProviderAli indicates Alibaba Cloud as cloud provider.
	CloudProviderAli CloudProvider = "ali"
	// CloudProviderOpenStack indicates OpenStack as cloud provider.
	CloudProviderOpenStack CloudProvider = "openstack"
)

// ClientMode indicates the connection mode of k8s client
type ClientMode string

const (
	NetworkClient ClientMode = "Network"
	InMemClient   ClientMode = "InMemory"
)

// ClientFacades is a holder for the primary k8s client and informer interfaces
type ClientFacades struct {
	Mode               ClientMode
	Client             kubernetes.Interface
	DynClient          dynamic.Interface
	InformerFactory    informers.SharedInformerFactory
	DynInformerFactory dynamicinformer.DynamicSharedInformerFactory
}
