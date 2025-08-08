// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingAdvisorConfiguration defines the configuration for the scalingadvisor operator.
type ScalingAdvisorConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// ClientConnection defines the configuration for constructing a kube client.
	ClientConnection componentbaseconfigv1alpha1.ClientConnectionConfiguration `json:"clientConnection"`
	// LeaderElection defines the configuration for leader election.
	LeaderElection componentbaseconfigv1alpha1.LeaderElectionConfiguration `json:"leaderElection"`
	// HealthProbes is the host and port for serving the healthz and readyz endpoints.
	// +optional
	HealthProbes *commontypes.HostPort `json:"healthProbes,omitempty"`
	// Metrics is the host and port for serving the metrics endpoint.
	// +optional
	Metrics *commontypes.HostPort `json:"metrics,omitempty"`
	// Profiling is the host and port for serving the profiling endpoints.
	Profiling *commontypes.HostPort `json:"profiling,omitempty"`
	// Server is basic server configuration for the scaling advisor.
	Server commontypes.ServerConfig `json:"server"`
	// Controllers defines the configuration for controllers.
	Controllers ControllersConfiguration `json:"controllers"`
}

// ControllersConfiguration defines the configuration for controllers that are run as part of the scaling-advisor.
type ControllersConfiguration struct {
	// ScalingConstraints is the configuration for then controller that reconciles ScalingConstraints.
	ScalingConstraints ScalingConstraintsControllerConfiguration `json:"scalingConstraints"`
}

// ScalingConstraintsControllerConfiguration is the configuration for then controller that reconciles ScalingConstraints.
type ScalingConstraintsControllerConfiguration struct {
	// ConcurrentSyncs is the maximum number concurrent reconciliations that can be run for this controller.
	ConcurrentSyncs *int `json:"concurrentSyncs"`
}
