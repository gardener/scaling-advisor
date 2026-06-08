package planner

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"

	corev1 "k8s.io/api/core/v1"
)

// ScaleInCandidateSelectorArgs carries the inputs a [ScaleInCandidateSelector] needs to
// evaluate the cluster view and pick the next scale-in candidate.
type ScaleInCandidateSelectorArgs struct {
	// View is the cluster view to enumerate nodes and their pods from.
	View minkapi.View
	// PDBTracker is consulted to skip nodes whose eviction would violate a PDB.
	PDBTracker PDBTracker
	// UtilizationThresholds bounds, per resource, the fraction (0-1) below which a node is
	// considered underutilized.
	UtilizationThresholds map[corev1.ResourceName]float64
	// Constraint carries the user's NodePool min/max and priority configuration.
	Constraint sacorev1alpha1.ScalingConstraintSpec
}

// ScaleInCandidateSelector enumerates scale-in candidates from a [minkapi.View], filtering on
// utilization, PDBs, and pool constraints.
type ScaleInCandidateSelector interface {
	// Init seeds the selector's candidate set from the view in args; called once per request
	// before NextCandidate.
	Init(ctx context.Context, args ScaleInCandidateSelectorArgs) error
	// NextCandidate returns the next candidate node, or (nil, nil) when none remain. args may
	// carry an updated view reflecting prior accepted candidates.
	NextCandidate(ctx context.Context, args ScaleInCandidateSelectorArgs) (*corev1.Node, error)
	// RemoveCandidateNode drops nodeName from the internal candidate pool so subsequent
	// NextCandidate calls do not return it.
	RemoveCandidateNode(nodeName string)
}

// NodeUtilizationCalculator computes the resource utilization of a node from the pods on it.
type NodeUtilizationCalculator interface {
	// GetUtilization returns each tracked resource's usage as a fraction of node allocatable.
	GetUtilization(node corev1.Node, pods []corev1.Pod) NodeUtilization
}

// NodeUtilization captures per-resource utilization derived from pod resource requests (not
// metrics-server data).
type NodeUtilization struct {
	// ResourceRatios maps a resource name (e.g. "cpu", "memory") to its utilization as a
	// fraction in [0, 1.0]; values above 1.0 indicate overcommit.
	ResourceRatios map[corev1.ResourceName]float64
}

// BelowResourceThreshold reports whether nu's utilization for resourceName is strictly below
// threshold. Resources missing from ResourceRatios are treated as below any threshold.
func (nu NodeUtilization) BelowResourceThreshold(resourceName corev1.ResourceName, threshold float64) bool {
	if utilization, exists := nu.ResourceRatios[resourceName]; exists {
		return utilization < threshold
	}
	return true
}

// BelowUtilizationThreshold reports whether nu is below watermark on every resource named in
// watermark. Resources present in nu but absent from watermark are ignored.
func (nu NodeUtilization) BelowUtilizationThreshold(watermark NodeUtilization) bool {
	for resourceName, threshold := range watermark.ResourceRatios {
		if !nu.BelowResourceThreshold(resourceName, threshold) {
			return false
		}
	}
	return true
}

// ScaleInSimArgs are the dependencies a [SimulationFactory] needs to construct a
// [ScaleInSimulation] for one request.
type ScaleInSimArgs struct {
	// SchedulerLauncher launches an in-process kube-scheduler bound to the simulation view.
	SchedulerLauncher SchedulerLauncher
	// RunCounter is shared across simulations in a request and increments per Run.
	RunCounter *atomic.Uint32
	// Name is the logical simulation name; surfaces in logs, traces, and results.
	Name string
	// NodeName is currently unused; the candidate node is supplied per-Run instead.
	NodeName string
	// TraceDir is the base directory for trace logs and per-run dumps. Empty disables tracing.
	TraceDir string
	// Config is the simulator-wide configuration (poll interval, thresholds, etc.).
	Config SimulatorConfig
}

// ScaleInSimulation simulates removing one candidate node at a time, rescheduling its
// displaced pods, and reporting whether the workload still fits.
type ScaleInSimulation interface {
	commontypes.Resettable
	// Name returns the logical simulation name.
	Name() string
	// Status returns the current [ActivityStatus].
	Status() ActivityStatus
	// PriorityKey returns the priority used by orchestration to order simulations.
	PriorityKey() commontypes.PriorityKey
	// Run executes one simulation pass for node against view. Blocks until the run stabilizes,
	// errors, or ctx is cancelled. Not safe for concurrent invocation on the same receiver.
	Run(ctx context.Context, view minkapi.View, node *corev1.Node) error
	// Result returns the most recent run's outcome:
	//   - the result when status is [ActivityStatusSuccess];
	//   - a sentinel error when still [ActivityStatusPending] or [ActivityStatusRunning];
	//   - the underlying failure error when [ActivityStatusFailure].
	Result() (ScaleInSimRunResult, error)
}

// ScaleInSimRunResult is the outcome of a completed [ScaleInSimulation.Run].
type ScaleInSimRunResult struct {
	// Name of the simulation that produced this result.
	Name string
	// View is the post-run view, including any mutations made during the run.
	View minkapi.View
	// Item describes the candidate node placement and name.
	Item sacorev1alpha1.ScaleInItem
	// IsSimulationSuccess is true iff every displaced pod was rescheduled and no
	// previously-scheduled pod was left unschedulable.
	IsSimulationSuccess bool
}

// ScaleInSimulator is the request-level facade: select candidates, run simulations, emit
// [ScaleInPlanResult]s.
type ScaleInSimulator interface {
	io.Closer
	// Simulate evaluates request against requestView and returns a channel that delivers one
	// or more results and is closed when no more will follow.
	Simulate(ctx context.Context, request *Request, requestView minkapi.View) <-chan ScaleInPlanResult
}

// ScaleInMemento carries cross-invocation timestamps so the planner can require sustained
// under-utilization before recommending a node for removal.
type ScaleInMemento struct {
	// LastIdentifiedUnneededNodes maps node name to the time it was first observed as a
	// successful scale-in candidate. The planner emits a node only after it has stayed in
	// this map for the configured underutilized duration.
	LastIdentifiedUnneededNodes map[string]time.Time
	// LastUnderutilizedSinceNodes maps node name to the last time it was observed below the
	// utilization thresholds, regardless of full-simulation outcome.
	LastUnderutilizedSinceNodes map[string]time.Time
}

// ScaleInPlanResult is one delivery from [ScaleInSimulator.Simulate].
type ScaleInPlanResult struct {
	// Error, when non-nil, is terminal; the channel is closed after delivering it.
	Error error `json:"error,omitempty"`
	// Labels is metadata propagated onto the planner response.
	Labels map[string]string `json:"labels,omitempty"`
	// ScaleInPlan is the generated plan, or nil when no candidates qualified (paired with
	// Error == [ErrNoScaleInPlan]).
	ScaleInPlan *sacorev1alpha1.ScaleInPlan `json:"scaleInPlan,omitempty"`
	// Memento captures cross-invocation state the planner persists for the next request.
	Memento ScaleInMemento `json:"scaleInMemento,omitempty"`
	// View is the post-simulation view, used by downstream stages (e.g. scale-out).
	View minkapi.View
}
