package planner

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/planner/pdb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ScaleInCandidateArgs encapsulates the arguments needed to select a candidate for scale in.
type ScaleInCandidateArgs struct {
	Constraint        sacorev1alpha1.ScalingConstraintSpec
	View              minkapi.View
	RequestRef        RequestRef
	PDBTracker        pdb.RemainingPdbTracker
	CandidateSelector ScaleInCandidateSelector
}

// ScaleInCandidateSelector is the interface meant to select the next viable candidate for scale in by interrogating the nodes from the [minkapi.View]
type ScaleInCandidateSelector interface {
	NextCandidate(ctx context.Context, args ScaleInCandidateArgs, skipNodes *sets.Set[string]) (*corev1.Node, error)
}

// NodeUtilizationCalculator is the interface meant to calculate the utilization of a node having the given node name in the [minkapi.View]
type NodeUtilizationCalculator interface {
	GetUtilization(context context.Context, view minkapi.View, nodeName string) (NodeUtilization, error)
}

// NodeUtilization is the utilization of all resources on a node expressed as a fraction from 0 to 1
type NodeUtilization struct {
	// ResourceRatios is a map of resource names such `cpu`/`memory`/`gpu`/etc. to the utilization expressed as fraction from [0-1.0]
	ResourceRatios map[corev1.ResourceName]float64
}

// BelowResourceThreshold returns true if the utilization of the given resource is below the provided threshold.
func (nu NodeUtilization) BelowResourceThreshold(resourceName corev1.ResourceName, threshold float64) bool {
	if utilization, exists := nu.ResourceRatios[resourceName]; exists {
		return utilization < threshold
	}
	return true
}

// BelowUtilizationThreshold returns true if the utilization of all resources is below the provided thresholds in the watermark.
func (nu NodeUtilization) BelowUtilizationThreshold(watermark NodeUtilization) bool {
	for resourceName, threshold := range watermark.ResourceRatios {
		if !nu.BelowResourceThreshold(resourceName, threshold) {
			return false
		}
	}
	return true
}

// ScaleInSimArgs represents the arguments necessary for creating a [ScaleInSimulation] instance.
type ScaleInSimArgs struct {
	// SchedulerLauncher is used to launch scheduler instances for the simulation.
	SchedulerLauncher SchedulerLauncher
	// RunCounter is an atomic counter for tracking simulation runs.
	RunCounter *atomic.Uint32
	// Name is the name of the simulation instance
	Name string
	// Config is the simulation configuration.
	Config ScaleInSimulatorConfig
	//NodeName is the name of the node to be simulated for scale in.
	NodeName string
	// TraceDir is the base directory for storing trace logs and other dump data by the simulation
	TraceDir string
}

// ScaleInSimulation represents a simulation that removes a virtual node(s) and attempts to bind the resulting evicted pods to already ready node
// in a minkapi View.
type ScaleInSimulation interface {
	commontypes.Resettable
	// Name returns the logical simulation name
	Name() string
	// Status returns the current ActivityStatus of the simulation
	Status() ActivityStatus
	// PriorityKey returns the PriorityKey for the [ScaleInSimulation] which represents the priority order in which a scale-in simulation is executed
	PriorityKey() commontypes.PriorityKey
	// Run executes the simulation against the given simulation [minkapi.View] to completion and returns any encountered error.
	// This is a blocking call, and callers are expected to manage concurrency and [ScaleInSimRunResult] consumption.
	Run(ctx context.Context, view minkapi.View, nodeName string) error
	// Result returns the latest [ScaleInPlanResult] if the simulation is in ActivityStatusSuccess,
	// or nil if the simulation is in ActivityStatusPending or ActivityStatusRunning
	// or an error if the ActivityStatus is ActivityStatusFailure
	Result() (ScaleInSimRunResult, error)
}

type ScaleInCandidateSelectorArgs struct{}

// ScaleInSimRunResult encapsulated the result of a completed [ScaleInSimulation]
type ScaleInSimRunResult struct {
	// Name of the ScaleInSimulation that produced this result.
	Name string
	// View is the [minkapi.View] against which the simulation was run.
	View minkapi.View
	// Items is the slice of [sacorev1alpha1.ScaleInItem] where each item encapsulates the
	// [sacorev1alpha1.NodePlacement] and associated delta.
	Items []sacorev1alpha1.ScaleInItem
	// PodsToReschedule Pods on Scaled-In node which are pending reschedule to other nodes.
	PodsToReschedule sets.Set[commontypes.NamespacedName]
	// NodePodAssignments represent the assignment of pods to reschedule to an existing nodes in the view other than the scaled in node.
	NodePodAssignments []NodePodAssignment
}

// ScaleInSimulator is a facade that executes [ScaleInSimulation]'s to generate one or more [ScaleInPlanResult]'s sent on a result channel.
type ScaleInSimulator interface {
	io.Closer

	Simulate(ctx context.Context, request *Request, simulationFactory SimulationFactory) <-chan ScaleInPlanResult
}

// ScaleInMemento is an encapsulation of partial details of a completed scale-in simulation that can be used by subsequent scale-in simulations
type ScaleInMemento struct {
	// LastIdentifiedUnneededNodes is a map of nodeName to the timestamp of when it was last successfully simulated for scale-in.
	// This can be used by subsequent simulations to skip simulating the same node again within a certain time window.
	LastIdentifiedUnneededNodes map[string]time.Time
	// LastUnderutilizedSinceNodes is a map of nodeName to the timestamp of when it was last successfully simulated for underutilization.
	LastUnderutilizedSinceNodes map[string]time.Time
}

// ScaleInPlanResult represents a result from the ScaleInSimulator.Simulate
type ScaleInPlanResult struct {
	// Error is any error encountered during plan generation. Represents a terminal error that occurred during plan generation
	// No further responses will be sent for the associated request.
	Error error `json:"error,omitempty"`
	// Labels is the associated metadata.
	Labels map[string]string `json:"labels,omitempty"`
	// ScaleInPlan is the generated scale-in plan.
	ScaleInPlan *sacorev1alpha1.ScaleInPlan `json:"scaleInPlan,omitempty"`
	//Memento is the partial details of a completed scale-in simulation
	Memento ScaleInMemento `json:"scaleInMemento,omitempty"`
}

// Note: Check with SimulatorConfig. SimulatorConfig will be changed to ScaleOutSimulatorConfig and the common fields between ScaleOutSimulatorConfig and ScaleInSimulatorConfig need to be looked at.
// ScaleInSimulatorConfig is static config params used to construct an instance of ScaleInSimulator
type ScaleInSimulatorConfig struct {
	// TrackPollInterval is the polling interval for tracking pod scheduling in the view of the simulator.
	TrackPollInterval time.Duration
	// UtilizationThresholds is the resource utilization thresholds for a node to be considered underutilized and thus
	// a candidate for scale in.
	// The keys are the resource names such as `cpu`/`memory`/`gpu`/etc. and the values are the corresponding
	// utilization thresholds expressed as fractions from 0 to 1.
	UtilizationThresholds map[corev1.ResourceName]float64
	// UnderutilizedDuration is the duration for which a node should be under the utilization thresholds to be
	// considered a candidate for scale in.
	UnderutilizedDuration time.Duration
	// MaxUnchangedTrackAttempts is the maximum number of unchanged simulation track attempts after which a simulation run is
	// considered as stabilized.
	MaxUnchangedTrackAttempts int
}
