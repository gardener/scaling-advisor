// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OperatorConfig defines the configuration for the scalingadvisor operator.
//
//nolint:govet // fieldalignment: intentional layout for k8s CRD / wire compatibility
type OperatorConfig struct {
	metav1.TypeMeta `json:",inline"`
	// Server is basic server configuration for the scaling advisor.
	Server ScalingAdvisorServerConfig `json:"server"`
	// AdviceGeneration contains configuration for scaling advice generation.
	AdviceGeneration ScalingAdviceGenerationConfig
	// CloudProvider specifies the cloud provider for which the scaling advisor is configured.
	CloudProvider commontypes.CloudProvider `json:"cloudProvider"`
	// ClientConnection defines the configuration for constructing a kube client.
	ClientConnection ClientConnectionConfig `json:"clientConnection"`
	// LeaderElection defines the configuration for leader election.
	LeaderElection LeaderElectionConfig `json:"leaderElection"`
	// Controllers defines the configuration for controllers.
	Controllers ControllersConfig `json:"controllers"`
}

// ClientConnectionConfig contains details for constructing a client.
type ClientConnectionConfig struct {
	// KubeConfigPath is the path to kube-config.
	KubeConfigPath string `json:"kubeConfigPath,omitempty"`
	// ContentType is the content type used when sending data to the server from this client.
	ContentType string `json:"contentType,omitempty"`
	// AcceptContentTypes defines the Accept header sent by clients when connecting to the server,
	// overriding the default value of 'application/json'. This field will control all connections
	// to the server used by a particular client.
	AcceptContentTypes string `json:"acceptContentTypes,omitempty"`
	// Burst allows extra queries to accumulate when a client is exceeding its rate.
	Burst int `json:"burst,omitempty"`
	// QPS controls the number of queries per second allowed for this connection.
	QPS float32 `json:"qps,omitempty"`
}

// LeaderElectionConfig defines the configuration for the leader election.
type LeaderElectionConfig struct {
	// ResourceLock determines which resource lock to use for leader election.
	// This is only applicable if leader election is enabled.
	ResourceLock string `json:"resourceLock"`
	// ResourceName determines the name of the resource that leader election
	// will use for holding the leader lock.
	// This is only applicable if leader election is enabled.
	ResourceName string `json:"resourceName"`
	// ResourceNamespace determines the namespace in which the leader
	// election resource will be created.
	// This is only applicable if leader election is enabled.
	ResourceNamespace string `json:"resourceNamespace"`
	// LeaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire
	// leadership of the occupied but un-renewed leader slot. This is effectively the
	// maximum duration that a leader can be stopped before it is replaced
	// by another candidate. This is only applicable if leader election is
	// enabled.
	LeaseDuration metav1.Duration `json:"leaseDuration"`
	// RenewDeadline is the interval between attempts by the acting leader to
	// renew its leadership before it stops leading. This must be less than or
	// equal to the lease duration.
	// This is only applicable if leader election is enabled.
	RenewDeadline metav1.Duration `json:"renewDeadline"`
	// RetryPeriod is the duration leader elector clients should wait
	// between attempting acquisition and renewal of leadership.
	// This is only applicable if leader election is enabled.
	RetryPeriod metav1.Duration `json:"retryPeriod"`
	// Enabled specifies whether leader election is enabled. Set this
	// to true when running replicated instances of the operator for high availability.
	Enabled bool `json:"enabled"`
}

// ScalingAdvisorServerConfig is the configuration for Scaling Advisor server.
type ScalingAdvisorServerConfig struct {
	// HealthProbesBindAddress is the host and port for serving health probes.
	HealthProbeBindAddress string `json:"healthProbeBindAddress,omitempty"`
	// Metrics is the host and port for serving metrics.
	MetricsBindAddress string `json:"metricsBindAddress,omitempty"`
	// ProfilingBindAddress is the host and port for serving profiling data.
	ProfilingBindAddress string `json:"profilingBindAddress,omitempty"`
	// ProfilingEnable indicates whether profiling is enabled.
	ProfilingEnabled bool `json:"profilingEnabled"`
}

// ScalingAdviceGenerationConfig contains configuration for scaling advice generation.
type ScalingAdviceGenerationConfig struct {
	// Mode defines the mode in which scaling advice is generated.
	Mode commontypes.ScalingAdviceGenerationMode `json:"mode"`
	// SimulationStrategy defines the simulation strategy to be used for scaling virtual nodes for generation of scaling advice.
	SimulationStrategy commontypes.SimulationStrategy `json:"simulationStrategy"`
	// ScoringStrategy defines the node scoring strategy to use for scaling decisions.
	ScoringStrategy commontypes.NodeScoringStrategy `json:"scoringStrategy"`
}

// ControllersConfig defines the configuration for controllers that are run as part of the scaling-advisor.
type ControllersConfig struct {
	// ScalingConstraints is the configuration for then controller that reconciles ScalingConstraints.
	ScalingConstraints ScalingConstraintsControllerConfig `json:"scalingConstraints"`
}

// ScalingConstraintsControllerConfig is the configuration for then controller that reconciles ScalingConstraints.
type ScalingConstraintsControllerConfig struct {
	// ConcurrentSyncs is the maximum number concurrent reconciliations that can be run for this controller.
	ConcurrentSyncs int `json:"concurrentSyncs"`
}
