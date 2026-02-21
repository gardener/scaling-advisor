package planner

import (
	"context"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"io"
	corev1 "k8s.io/api/core/v1"
	"sync/atomic"
)

// ScaleOutSimulator is a facade that executes simulations to generate one or more scale-out plans.
// Implementations vary depending on the commontypes.SimulatorStrategy used.
//
// Depending upon the SimulatorStrategy, the implementation creates and organizes ScaleOutSimulation into ScaleOutSimGroup's differently.
//
//	(TODO: extend the example below with two availability zones - currently a single default one is assumed)
//
// SimulatorStrategySingleNodeMultiSim
//
//	ScalingConstraints
//		np-a: 1 {nt-a: 1, nt-b: 2, nt-c: 1}
//		np-b: 2 {nt-q: 2, nt-r: 1, nt-s: 1}
//	SimulationGroups
//		g1: {PoolPriority: 1, NTPriority: 1, nt-a, nt-c}
//		g2: {PoolPriority: 1, NTPriority: 2, nt-b}
//		g3: {PoolPriority: 2, NTPriority: 1, nt-r, nt-s}
//		g4: {PoolPriority: 2, NTPriority: 2, nt-q}
//
// SimulatorStrategyMultiNodeSingleSim
//
//	np-a: 1 {nt-a: 1, nt-b: 2, nt-c: 1}
//	np-b: 2 {nt-q: 2, nt-r: 1, nt-s: 1}
//	np-c: 1 {nt-x: 2, nt-y: 1}
//
//	g1: {PoolPriority: 1, NTPriority: 1, nt-a, nt-c, nt-y}
//	g2: {PoolPriority: 1, NTPriority: 2, nt-b, nt-x}
//	g3: {PoolPriority: 2, NTPriority: 1, nt-r, nt-s}
//	g4: {PoolPriority: 2, NTPriority: 2, nt-q}
//
// An implementation created for the SimulatorStrategySingleNodeMultiSim, will run the different ScaleOutSimulation's of a ScaleOutSimGroup
// concurrently where each simulation Run virtually scales ONE one node in its MinKAPI overlay View for a combination of NodePool, NodeTemplate and
// AvailabilityZone. The configured SchedulerLauncher is used to launch embedded Scheduler which does pod assignments to the virtual scaled node.
// This concludes one "Run" of the simulation.
// The scaled node which is the "winner" of this pass
//
// Or may run a single
// simulation by scaling
// multiple nodes for a given
// group for all combinations of NodePool, NodeTemplate and AvailabilityZone. Simulations for a group are run before moving
// to the group at the next priority level. Moving to the next group is only done if there are leftover unscheduled pods after
// running all simulations in the current group.
type ScaleOutSimulator interface {
	io.Closer
	// Simulate is the high level activity that
	//  - Creates ScaleOutSimulation's using the given SimulationFactory
	//  - Organizes ScaleOutSimulation's into ScaleOutSimGroup's according to priority and the configured SimulatorStrategy.
	//  - Executes each ScaleOutSimGroup until stabilization, collecting ScaleOutSimResult's and aggregating them into SimulationGroupRunResult
	//  - The SimulationGroupRunResult is c
	//  - invoke the NodeScorer to determine a winner NodeScore
	//
	// the simulator specific SimulatorStrategy to generate one or more ScaleOutPlan's each encapsulated within a
	// ScaleOutPlanResult that is offered on the resultCh channel.
	Simulate(ctx context.Context, request *Request, simulationFactory SimulationFactory) (planResult <-chan ScaleOutPlanResult)
}

// SimulatorFactory is a factory facade for constructing various kinds of simulators.
type SimulatorFactory interface {
	GetScaleOutSimulator(args SimulatorArgs) (ScaleOutSimulator, error)
	// TODO: Add GetScaleInSimulator here.
}

// SimulationFactory is a factory facade for creating Simulation objects
type SimulationFactory interface {
	// NewScaleOut creates a ScaleOutSimulation instance with the given name and arguments.
	NewScaleOut(name string, args ScaleOutSimArgs) (ScaleOutSimulation, error)
	// TODO: Add NewScaleIn method here.
}

// ScaleOutPlanResult represents a result from the ScaleOutSimulator.Simulate
type ScaleOutPlanResult struct {
	// Error is any error encountered during plan generation. Represents a terminal error that occurred during plan generation
	// No further responses will be sent for the associated request.
	Error error `json:"error,omitempty"`
	// Labels is the associated metadata.
	Labels map[string]string `json:"labels,omitempty"`
	// ScaleOutPlan is the generated scale-out plan.
	ScaleOutPlan *sacorev1alpha1.ScaleOutPlan `json:"scaleOutPlan,omitempty"`
}

// ScaleOutSimulation represents a simulation that scales virtual node(s) and performs valid unscheduled pod to ready node
// assignments against a minkapi View.
// A ScaleOutSimulation implementation may use a k8s scheduler - either embedded or external to do this, or it may form a SAT/MIP model
// from the pod/node data and run a tool that solves the model.
type ScaleOutSimulation interface {
	commontypes.Resettable
	// Name returns the logical simulation name
	Name() string
	// ActivityStatus returns the current ActivityStatus of the simulation
	ActivityStatus() ActivityStatus
	// PriorityKey returns the PriorityKey for the simulation which is the key by which simulations are grouped and determines
	// the order in which simualtions are run.
	PriorityKey() PriorityKey
	// Run executes the simulation against the given view to completion and returns any encountered error.
	// This is a blocking call, and callers are expected to manage concurrency and ScaleOutSimResult consumption.
	Run(ctx context.Context, view minkapi.View) error
	// Result returns the latest ScaleOutSimResult if the simulation is in ActivityStatusSuccess,
	// or nil if the simulation is in ActivityStatusPending or ActivityStatusRunning
	// or an error if the ActivityStatus is ActivityStatusFailure
	Result() (ScaleOutSimResult, error)
}

// ScaleOutSimResult contains the results of a completed simulation run.
type ScaleOutSimResult struct {
	// Name of the ScaleOutSimulation that produced this result.
	Name string
	// View is the minkapi View against which the simulation was run.
	View minkapi.View
	// ScaledNodePlacements represents the placement information for the scaled Nodes.
	ScaledNodePlacements []sacorev1alpha1.NodePlacement
	// ScaledNodePodAssignments represents the assignment of Pods to simulated scaled Nodes.
	ScaledNodePodAssignments []NodePodAssignment
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of the ClusterSnapshot
	// or it is a winning simulated Node from a previous run.
	OtherNodePodAssignments []NodePodAssignment
	// LeftoverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after simulation is completed.
	LeftoverUnscheduledPods []commontypes.NamespacedName
}

// ScaleOutSimArgs represents the arguments necessary for creating a scale-out simulation instance.
type ScaleOutSimArgs struct {
	// SchedulerLauncher is used to launch scheduler instances for the simulation.
	SchedulerLauncher SchedulerLauncher
	// StorageMetaAccess is interrogated for metadata to create CSINodes for the simulation
	StorageMetaAccess StorageMetaAccess
	// NodePool is the target node pool for the simulation.
	NodePool *sacorev1alpha1.NodePool
	// RunCounter is an atomic counter for tracking simulation runs.
	RunCounter *atomic.Uint32
	// AvailabilityZone is the target availability zone for the simulation.
	AvailabilityZone string
	// NodeTemplateName is the name of the node template to use in the simulation.
	NodeTemplateName string
	// Config is the simulation configuration.
	Config SimulatorConfig
	// TraceDir is the base directory for storing trace logs and other dump data by the simulation
	TraceDir string
}

// ScaleOutSimGroup is a group of ScaleOutSimulation's at the same priority level (ie a partition of simulations).
type ScaleOutSimGroup interface {
	commontypes.Resettable
	// Name returns the name of the simulation group.
	Name() string
	// GetKey returns the simulation group key.
	GetKey() PriorityKey
	// GetSimulations returns all simulations in this group.
	GetSimulations() []ScaleOutSimulation
	// AddSimulation adds a simulation to the group.
	AddSimulation(simulation ScaleOutSimulation)
	// Run executes all simulations in the group and returns all the simulation run results or any error.
	Run(ctx context.Context, getViewFn minkapi.ViewFactory) ([]ScaleOutSimResult, error)
}

// ScaleOutSimGroupPassScores represents the scoring results, including the winner score, for a single pass of a ScaleOutSimGroup
// after running the NodeScorer against the ScaleOutSimResult's of the pass.
type ScaleOutSimGroupPassScores struct {
	// WinnerScore is the highest scoring node in the group.
	WinnerScore *NodeScore
	// WinnerNode is the actual node corresponding to the winner score.
	WinnerNode *corev1.Node
	// AllScores contains all computed node scores for the group.
	AllScores []NodeScore
}

// ScaleOutSimGroupCycleResult represents the result of running all passes for a ScaleOutSimGroup.
type ScaleOutSimGroupCycleResult struct {
	// NextGroupPassView is the updated view after executing all passes in this group.
	// The next group, if any, should use this view as its base view for its overlay view.
	NextGroupPassView minkapi.View
	// Name is the name of the simulation group.
	Name string
	// WinnerNodeScores contains the node scores of the winning nodes.
	WinnerNodeScores []NodeScore
	// LeftoverUnscheduledPods contains the namespaced names of pods that could not be scheduled.
	LeftoverUnscheduledPods []commontypes.NamespacedName
	// PassNum is the number of passes executed in this group before moving to the next group.
	// A pass is defined as the execution of all simulations in a group.
	PassNum int
}
