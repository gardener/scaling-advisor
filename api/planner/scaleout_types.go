// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"io"
	"sync/atomic"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"

	corev1 "k8s.io/api/core/v1"
)

// ScaleOutSimulator is a facade that executes [ScaleOutSimulation]'s to generate one or more [ScaleOutPlanResult]'s sent
// on a result channel.
// Implementations vary depending on the [commontypes.SimulatorStrategy] used.
//
// Depending upon the implementation creates and organizes [ScaleOutSimulation]'s into [ScaleOutSimGroup]'s differently.
//
//	ScalingConstraints (Legend: pa -> "Pool-A", "ta" -> "Node Template A", "zx" -> "Zone X"
//		{pa:1, {ta: 1, tb: 2, tc: 1}, {zx, zy}}
//		{pb:2, {tq: 2, tr: 1, ts: 1}, {zz}}
//	groups
//		g1: PriorityKey(2,2): [ {pb, tq, zz} ]
//		g2: PriorityKey(2,1): [ {pb, tr, zz}, {pb, ts, zz}]
//		g3: PriorityKey(1,2): [ {pa, tb, zx}, {pa, tb, zy}]
//		g1: PriorityKey(1,1): [ {pa, ta, zx}, {pa, ta, zy}, {pa, tc, zx}, {pa, tc, zy} ]
//
// An [ScaleOutSimulator] implementation created, will do the
// following when Simulate is invoked:
//   - Creates [ScaleOutSimulation]'s using the [SimulationFactory] given as parameter
//   - Organizes [ScaleOutSimulation]'s into [ScaleOutSimGroup]'s according to [PriorityKey] and [commontypes.SimulatorStrategy]
//   - Executes each ScaleOutSimGroup until stabilization, collecting ScaleOutSimResult's and aggregating them into SimulationGroupRunResult
//   - The SimulationGroupRunResult is c
//   - invoke the NodeScorer to determine a winner NodeScore
//
// ill run the different
// [ScaleOutSimulation]'s of a [ScaleOutSimGroup] concurrently where each simulation Run virtually scales ONE one node in
// its MinKAPI overlay View for a [ScaleOutNodeTemplate] triple. The configured SchedulerLauncher is used to launch embedded
// `kube-scheduler` which does pod binding to the virtual scale-up node. This concludes one "Run" of the
//
// simulation.
//
// The scaled node which is the "winner" of this pass
//
// the simulator specific SimulatorStrategy to generate one or more ScaleOutPlan's each encapsulated within a
// ScaleOutPlanResult that is offered on the resultCh channel.
//
// Or may run a single
// simulation by scaling
// multiple nodes for a given
// group for all combinations of NodePool, NodeTemplate and AvailabilityZone. Simulations for a group are run before moving
// to the group at the next priority level. Moving to the next group is only done if there are leftover unscheduled pods after
// running all simulations in the current group.
type ScaleOutSimulator interface {
	io.Closer

	// Simulate is the high level activity that runs [ScaleOutSimulation] created from given
	// [SimulationFactory] with the given planner [Request].
	Simulate(ctx context.Context, request *Request, simulationFactory SimulationFactory) (planResult <-chan ScaleOutPlanResult)
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
// bindings against a minkapi View.
// The default ScaleOutSimulation implementation uses an embedded k8s scheduler to perform this work.
// More exotic implementations could form a SAT (Satisfiability Testing) /MIP (Mixed Integer Programming)
// constraint model from the pod/node data and run a tool that solves the model.
type ScaleOutSimulation interface {
	commontypes.Resettable
	// Name returns the logical simulation name
	Name() string
	// Status returns the current ActivityStatus of the simulation
	Status() ActivityStatus
	// PriorityKey returns the PriorityKey for the simulation which is the key by which simulations are grouped and determines
	// the order in which simulations are run.
	PriorityKey() commontypes.PriorityKey
	// Run executes the simulation against the given simulation [minkapi.View] to completion and returns any encountered error.
	// This is a blocking call, and callers are expected to manage concurrency and ScaleOutSimResult consumption.
	Run(ctx context.Context, view minkapi.View) error
	// Result returns the latest ScaleOutSimResult if the simulation is in ActivityStatusSuccess,
	// or nil if the simulation is in ActivityStatusPending or ActivityStatusRunning
	// or an error if the ActivityStatus is ActivityStatusFailure
	Result() (ScaleOutSimResult, error)
}

// ScaleOutSimArgs represents the arguments necessary for creating a [ScaleOutSimulation] instance.
type ScaleOutSimArgs struct {
	// SchedulerLauncher is used to launch scheduler instances for the simulation.
	SchedulerLauncher SchedulerLauncher
	// StorageMetaAccess is interrogated for metadata to create CSINodes for the simulation
	StorageMetaAccess StorageMetaAccess
	// RunCounter is an atomic counter for tracking simulation runs.
	RunCounter *atomic.Uint32
	// Name is the name of the simulation instance
	Name string
	// TraceDir is the base directory for storing trace logs and other dump data by the simulation
	TraceDir string
	// Strategy is the strategy being used by the parent [ScaleOutSimulator] that is running this simulation.
	Strategy commontypes.SimulatorStrategy
	// NodeTemplates is a slice of [ScaleOutNodeTemplate] representing information needed to create scale-out simulated nodes.
	NodeTemplates []ScaleOutNodeTemplate
	// Config is the simulation configuration.
	Config SimulatorConfig
}

// ScaleOutNodeTemplate is a superset of the [sacorev1alpha1.NodePlacement] consisting of enough information to create
// a simulated scale-out [corev1.Node] within a [minkapi.View] such that the `kube-scheduler` can bind pods to nodes.
//
// Depending on the choice of [commontypes.SimulatorStrategy], a [ScaleOutSimulation] can either:
//   - Execute multiple concurrent simulations scaling a node for each [ScaleOutNodeTemplate] at the same priority and choosing
//     a winner among the concurrent simulations to determine chosen [sacorev1alpha1.NodePlacement]'s in the [sacorev1alpha1.ScaleOutPlan]
//   - Or it may execute a single simulation scaling multiple nodes for all ScaleOutNodeTemplate's at same priority, choosing nodes
//     with successful pod-assignments to determine [sacorev1alpha1.NodePlacement]'s in the [sacorev1alpha1.ScaleOutPlan]
type ScaleOutNodeTemplate struct {
	// Labels is a map of key/value pairs for labels applied to all the nodes in this node pool.
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations is a map of key/value pairs for annotations applied to all the nodes in this node pool.
	Annotations map[string]string `json:"annotations,omitempty"`
	// Quota defines the resource quota for the node pool.
	Quota corev1.ResourceList `json:"quota,omitempty"`
	// Capacity defines the capacity for node resources that are available for the node's instance type.
	Capacity corev1.ResourceList `json:"capacity"`
	// KubeReserved defines the capacity for kube reserved resources.
	// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#kube-reserved for additional information.
	KubeReserved corev1.ResourceList `json:"kubeReservedCapacity,omitempty"`
	// SystemReserved defines the capacity for system reserved resources.
	// See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#system-reserved for additional information.
	SystemReserved               corev1.ResourceList `json:"systemReservedCapacity,omitempty"`
	sacorev1alpha1.NodePlacement `json:",inline"`
	// Architecture is the CPU architecture of the node's instance type.
	Architecture string `json:"architecture"`
	// Taints is a list of taints applied to all the nodes in this node pool.
	Taints []corev1.Taint `json:"taints,omitempty"`
	// PriorityKey is the priority key for this ScaleOutNodeTemplate.
	PriorityKey commontypes.PriorityKey
}

// ScaleOutSimResult contains the results of a completed simulation run.
type ScaleOutSimResult struct {
	// Name of the ScaleOutSimulation that produced this result.
	Name string
	// View is the minkapi View against which the simulation was run.
	View minkapi.View
	// Items is the slice of [sacorev1alpha1.ScaleOutItem] where each item encapsulates the
	// [sacorev1alpha1.NodePlacement] and associated delta.
	Items []sacorev1alpha1.ScaleOutItem
	// NodePodAssignments represents the assignment of Pods to simulated scale-out Nodes.
	NodePodAssignments []NodePodAssignment
	// OtherNodePodAssignments represent the assignment of unscheduled Pods to either an existing Node which is part of
	// the ClusterSnapshot, or it is a simulated scale-out Node from a previous run.
	OtherNodePodAssignments []NodePodAssignment
	// LeftoverUnscheduledPods is the slice of unscheduled pods that remain unscheduled after the simulation Run is
	// completed.
	LeftoverUnscheduledPods []commontypes.NamespacedName
}

// ScaleOutSimGroup is a group of ScaleOutSimulation's at the same priority level (ie a partition of simulations).
type ScaleOutSimGroup interface {
	commontypes.Resettable
	// Name returns the name of the simulation group.
	Name() string
	// GetKey returns the simulation group key.
	PriorityKey() commontypes.PriorityKey
	// GetSimulations returns all simulations in this group.
	GetSimulations() []ScaleOutSimulation
	// AddSimulation adds a simulation to the group.
	AddSimulation(simulation ScaleOutSimulation)
	// Run executes all simulations in the group and returns all the simulation run results or any error.
	Run(ctx context.Context, getViewFn minkapi.GetViewFunc) ([]ScaleOutSimResult, error)
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

// CmpScaleOutSimulationDecreasingPriority is a cmp function for [ScaleOutSimulation] that compares by decreasing PriorityKey.
func CmpScaleOutSimulationDecreasingPriority(s1, s2 ScaleOutSimulation) int {
	return commontypes.CmpPriorityKeyDecreasing(s1.PriorityKey(), s2.PriorityKey())
}

// CmpScaleOutSimGroup is a cmp function for [ScaleOutSimGroup] that compares by decreasing PriorityKey.
func CmpScaleOutSimGroup(s1, s2 ScaleOutSimGroup) int {
	return commontypes.CmpPriorityKeyDecreasing(s1.PriorityKey(), s2.PriorityKey())
}

// CmpScaleOutNodeTemplate is a cmp function for [ScaleOutNodeTemplate] that compares by decreasing PriorityKey.
func CmpScaleOutNodeTemplate(a, b ScaleOutNodeTemplate) int {
	return commontypes.CmpPriorityKeyDecreasing(a.PriorityKey, b.PriorityKey)
}
