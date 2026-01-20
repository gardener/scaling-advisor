// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

// Resettable defines types that can reset their state to a default or initial configuration.
type Resettable interface {
	// Reset resets the state of the implementing type.
	Reset()
}

// Service is a component that can be started and stopped.
type Service interface {
	// Start starts the core with the given context. Start may block depending on the implementation - if the core is a server.
	// The context is expected to be populated with a logger.
	Start(ctx context.Context) error
	// Stop stops the core. Stop does not block.
	Stop(ctx context.Context) error
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

// QPSBurst is a simple encapsulation of client QPS and Burst settings.
type QPSBurst struct {
	// QPS is the queries per second rate limit for the client.
	QPS float32 `json:"qps"`
	// Burst is the burst size for rate limiting, allowing temporary spikes above QPS.
	Burst int `json:"burst"`
}

// ConstraintReference is a reference to the ClusterScalingConstraint for which this advice is generated.
type ConstraintReference struct {
	// Name is the name of the ClusterScalingConstraint.
	Name string `json:"name"`
	// Namespace is the namespace of the ClusterScalingConstraint.
	Namespace string `json:"namespace"`
}

// SimulationStrategy represents a simulation strategy variant.
// +enum
type SimulationStrategy string

const (
	// SimulationStrategyMultiSimulationsPerGroup represents a simulation strategy which runs independent multiple simulations differentiated by scaling a node for a combination
	// of NodePool, NodeTemplate and AvailabilityZone.
	SimulationStrategyMultiSimulationsPerGroup SimulationStrategy = "multi-simulations-per-group"
	// SimulationStrategySingleSimulationPerGroup represents a simulation strategy which runs a single simulation by scaling multiple nodes for a given
	// group for all combinations of NodePool, NodeTemplate and AvailabilityZone.
	SimulationStrategySingleSimulationPerGroup SimulationStrategy = "single-simulation-per-group"
)

// ScalingAdviceGenerationMode defines the mode in which scaling advice is generated.
// +enum
type ScalingAdviceGenerationMode string

const (
	// ScalingAdviceGenerationModeIncremental is the mode in which scaling advice is generated incrementally.
	// In this mode, scaling advisor will dish out scaling advice as soon as it has the first scale-out/in advice from a simulation run.
	ScalingAdviceGenerationModeIncremental ScalingAdviceGenerationMode = "incremental"
	// ScalingAdviceGenerationModeAllAtOnce is the mode in which scaling advice is generated all at once.
	// In this mode, scaling advisor will generate scaling advice after it has run the complete set of simulations wher either
	// all pending pods have been scheduled or stabilised.
	ScalingAdviceGenerationModeAllAtOnce ScalingAdviceGenerationMode = "all-at-once"
)

// SupportedAdviceGenerationModes is a set of all supported scaling advice generation modes.
var SupportedAdviceGenerationModes = sets.New(
	ScalingAdviceGenerationModeIncremental,
	ScalingAdviceGenerationModeAllAtOnce,
)

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

// AsCloudProvider converts a string to CloudProvider type. It returns an error if the cloudProvider string
// is not supported.
func AsCloudProvider(cloudProvider string) (CloudProvider, error) {
	switch cloudProvider {
	case "aws":
		return CloudProviderAWS, nil
	case "gcp":
		return CloudProviderGCP, nil
	case "azure":
		return CloudProviderAzure, nil
	case "ali":
		return CloudProviderAli, nil
	case "openstack":
		return CloudProviderOpenStack, nil
	default:
		return "", fmt.Errorf("unuspported cloud provider: %s", cloudProvider)
	}
}

// ClientAccessMode indicates the access mode of k8s client
// +enum
type ClientAccessMode string

const (
	// ClientAccessModeNetwork indicates the client accesses k8s api-server via a network call.
	ClientAccessModeNetwork ClientAccessMode = "network"
	// ClientAccessModeInMemory indicates the client accesses k8s api-server via in-memory calls by passing network calls
	// thus reducing the need for serialization and deserialization of requests and responses.
	ClientAccessModeInMemory ClientAccessMode = "in-memory"
)

// ClientFacades is a holder for the primary k8s client and informer interfaces.
type ClientFacades struct {
	// Client is the standard Kubernetes clientset for accessing core APIs.
	Client kubernetes.Interface
	// DynClient is the dynamic client for accessing arbitrary Kubernetes resources.
	DynClient dynamic.Interface
	// InformerFactory provides shared informers for core Kubernetes resources.
	InformerFactory informers.SharedInformerFactory
	// DynInformerFactory provides shared informers for dynamic Kubernetes resources.
	DynInformerFactory dynamicinformer.DynamicSharedInformerFactory
	// Mode indicates the access mode of the Kubernetes client.
	Mode ClientAccessMode
}
