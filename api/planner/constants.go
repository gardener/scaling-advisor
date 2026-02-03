package planner

import (
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// DefaultTrackPollInterval is the default polling interval for tracking pod scheduling in the view of the simulator.
	DefaultTrackPollInterval = 20 * time.Millisecond
	// DefaultMaxParallelSimulations is the default maximum number of parallel simulations that can be run by the scaling advisor simulator.
	DefaultMaxParallelSimulations = 1
	//DefaultMaxUnchangedTrackAttempts is the default value for the maximum number of unchanged simulation track attempts before
	// a simulation run is considered as stabilized.
	DefaultMaxUnchangedTrackAttempts = 3
	// ServiceName is the program binary name for the independent scaling planner microservice.
	ServiceName = "scalp"
)

var (
	// RequiredNodeLabelNames is the set of label names that are required on any corev1.Node or NodeInfo object for the planner to be able to work with the node.
	RequiredNodeLabelNames = sets.New[string](
		corev1.LabelInstanceTypeStable,
		corev1.LabelArchStable,
		corev1.LabelTopologyZone,
		corev1.LabelTopologyRegion,
		corev1.LabelHostname,
		corev1.LabelInstanceTypeStable,
		commonconstants.LabelNodePoolName,
		commonconstants.LabelNodeTemplateName,
	)
)
