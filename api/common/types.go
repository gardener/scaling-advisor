// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"cmp"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

// Resettable defines types that can reset their state to a default or initial configuration.
type Resettable interface {
	// Reset resets the state of the implementing type.
	Reset() error
}

// QPSBurst is a simple encapsulation of client QPS and Burst settings.
type QPSBurst struct {
	// QPS is the queries per second rate limit for the client.
	QPS float32 `json:"qps"`
	// Burst is the burst size for rate limiting, allowing temporary spikes above QPS.
	Burst int `json:"burst"`
}

// NamespacedName is a fully qualified object name.
// NOTE: This is only needed since k8s APIMachinery types.NamespacedName does not have JSON tags and k8s maintainers
// recommended that every project should use their own copy of NamespacedName.
type NamespacedName struct {
	// Namespace is the namespace of the object.
	Namespace string `json:"namespace,omitempty"`
	// Name is the name of the object.
	Name string `json:"name"`
}

// AsObjectName converts this namespaced name to a client-go cache.ObjectName
func (nn NamespacedName) AsObjectName() cache.ObjectName {
	return cache.ObjectName{Name: nn.Name, Namespace: nn.Namespace}
}

// AsObjectReference constructs an ObjectReference referring this name or nil if name is empty.
func (nn NamespacedName) AsObjectReference() *corev1.ObjectReference {
	if nn.Name == "" {
		return nil
	} else {
		return &corev1.ObjectReference{Namespace: nn.Namespace, Name: nn.Name}
	}
}

// String returns the general purpose string representation.
// Matches implementation in APIMachinery types.NamespacedName
func (nn NamespacedName) String() string {
	return nn.Namespace + string(types.Separator) + nn.Name
}

// SimulatorStrategy represents a simulation strategy variant.
// +enum
type SimulatorStrategy string

const (
	// SimulatorStrategySingleNodeMultiSim represents a simulator strategy which runs independent multiple simulations differentiated by scaling a single node for a combination
	// of NodePool, NodeTemplate and AvailabilityZone.
	SimulatorStrategySingleNodeMultiSim SimulatorStrategy = "single-node-multi-sim"
	// SimulatorStrategyMultiNodeSingleSim represents a simulator strategy which runs a single simulation by scaling multiple nodes for
	// all combinations of NodePool, NodeTemplate and AvailabilityZone.
	SimulatorStrategyMultiNodeSingleSim SimulatorStrategy = "multi-node-single-sim"
)

// IsMultiNode returns true if the strategy scales multiple nodes, false otherwise.
func (s SimulatorStrategy) IsMultiNode() bool {
	return s == SimulatorStrategyMultiNodeSingleSim
}

// IsSingleNode returns true if the strategy scales only a single node, false otherwise.
func (s SimulatorStrategy) IsSingleNode() bool {
	return s == SimulatorStrategySingleNodeMultiSim
}

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

// IsIncremental returns true if the advice generation mode is incremental.
func (a ScalingAdviceGenerationMode) IsIncremental() bool {
	return a == ScalingAdviceGenerationModeIncremental
}

// IsAllAtOnce returns true if the advice generation mode is all-at-once.
func (a ScalingAdviceGenerationMode) IsAllAtOnce() bool {
	return a == ScalingAdviceGenerationModeAllAtOnce
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
		return "", fmt.Errorf("%w: unknown %q", commonerrors.ErrUnsupportedCloudProvider, cloudProvider)
	}
}

// ContextKey is the type alias for scaling advisor related context keys
type ContextKey string

const (
	// VerbosityCtxKey is the context key indicating the diagnostic/log verbosity.
	VerbosityCtxKey ContextKey = "verbosity"

	// TraceDirCtxKey is the context key under which the dir path to the trace dir is stored.
	TraceDirCtxKey ContextKey = "trace-dir"

	// TraceLogPathCtxKey is the context key under which the path to the trace log file is stored.
	TraceLogPathCtxKey ContextKey = "trace-log"
)

// PriorityKey represents a composite and comparable key for ordering objects that have a primary and secondary unit priority levels.
type PriorityKey struct {
	// First is the first priority value. Higher values represent higher priority.
	First int32
	// Second is the second priority value. Higher value represent higher priority. Secondary weight compared to First
	Second int32
}

// String returns a string representation of the PriorityKey.
func (k PriorityKey) String() string {
	return fmt.Sprintf("%d-%d", k.First, k.Second)
}

// CmpPriorityKeyDecreasing is a compare function for [PriorityKey] in decreasing value of priority.
// ie higher priority values  before lower priority values which is the kubernetes convention.
func CmpPriorityKeyDecreasing(a, b PriorityKey) int {
	if firstCmp := cmp.Compare(b.First, a.First); firstCmp != 0 {
		return firstCmp
	}
	return cmp.Compare(b.Second, a.Second)
}
