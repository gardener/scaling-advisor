package planner

import (
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// DefaultMaxParallelSimulations is the default maximum number of parallel simulations that can be run by the scaling advisor simulator.
	DefaultMaxParallelSimulations = 1
	// DefaultTrackPollInterval is the default polling interval for tracking pod scheduling in the view of the simulator.
	DefaultTrackPollInterval = 40 * time.Millisecond
	//DefaultMaxUnchangedTrackAttempts is the default value for the maximum number of unchanged simulation track attempts after which
	// a simulation run is considered as stabilized.
	DefaultMaxUnchangedTrackAttempts = 65
	// ServiceName is the program binary name for the independent scaling planner microservice.
	ServiceName = "scaling-planner"
)

var (
	// RequiredNodeLabelNames is the set of label names that are required on any corev1.Node or NodeInfo object for the planner to be able to work with the node.
	RequiredNodeLabelNames = sets.New[string](
		corev1.LabelInstanceTypeStable,
		corev1.LabelArchStable,
		corev1.LabelTopologyZone,
		corev1.LabelTopologyRegion,
		corev1.LabelHostname,
		commonconstants.LabelNodePoolName,
		commonconstants.LabelNodeTemplateName,
	)
)
