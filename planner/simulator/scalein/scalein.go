package scalein

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/go-logr/logr"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ plannerapi.ScaleInSimulator = (*defaultSimulator)(nil)

type defaultSimulator struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	traceDir          string
	state             RequestState
	simulatorConfig   plannerapi.ScaleInSimulatorConfig
}

func (d *defaultSimulator) Close() error {
	return d.state.Reset()
}

// Simulate implements [plannerapi.ScaleInSimulator.Simulate]. It iteratively selects scale-in candidates,
// runs a simulation for each candidate, and accumulates successfully scaled-in nodes. A node is only included
// in the final [plannerapi.ScaleInPlanResult] if it has been continuously identified as unneeded across invocations
// for at least the configured UnderutilizedDuration (tracked via the [plannerapi.ScaleInMemento]).
func (d *defaultSimulator) Simulate(ctx context.Context, requestRef plannerapi.RequestRef, memento *plannerapi.ScaleInMemento, requestView minkapi.View, factory plannerapi.SimulationFactory) <-chan plannerapi.ScaleInPlanResult {
	d.state = RequestStateWith(nil, d.simulatorConfig, factory, d.viewAccess)
	go func() {
		defer close(d.state.ResultCh)
		result := d.doSimulate(ctx, requestRef, memento, requestView, factory)
		d.state.ResultCh <- result
	}()
	return d.state.ResultCh
}

func (d *defaultSimulator) doSimulate(ctx context.Context, requestRef plannerapi.RequestRef, memento *plannerapi.ScaleInMemento, requestView minkapi.View, factory plannerapi.SimulationFactory) plannerapi.ScaleInPlanResult {
	log := logr.FromContextOrDiscard(ctx)

	// Create the scale-in simulation.
	simArgs := plannerapi.ScaleInSimArgs{
		SchedulerLauncher: d.schedulerLauncher,
		RunCounter:        d.state.SimRunCounter,
		Name:              "scalein-sim",
		Config:            d.simulatorConfig,
	}
	scaleInSim, err := factory.NewScaleIn(simArgs)
	if err != nil {
		return sendPlanError(requestRef, fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulation, err))
	}

	// Initialize views and loop state.
	simView := requestView
	lastScaleInSuccessView := requestView
	scaledInSuccessNodes := sets.New[string]()
	skipNodes := sets.New[string]()

	// Begin candidate selection loop.
	for {
		select {
		case <-ctx.Done():
			return sendPlanError(requestRef, ctx.Err())
		default:
			// Select the next scale-in candidate.
			nextCandidate, err := nextCandidate(ctx, plannerapi.ScaleInCandidateArgs{
				Constraint: sacorev1alpha1.ScalingConstraintSpec{}, // FIXME: pass actual constraint
				View:       simView,
				RequestRef: requestRef,
				SkipNodes:  skipNodes,
			})
			if err != nil {
				return sendPlanError(requestRef, fmt.Errorf("failed to select scale-in candidate: %w", err))
			}
			if nextCandidate == nil {
				log.V(3).Info("No more scale-in candidates available, ending simulation loop.")
				// Compute ScaleInPlanResult.
				return d.computePlanResult(log, memento, scaledInSuccessNodes, requestRef)
			}

			candidateName := nextCandidate.Name
			log.V(3).Info("Running scale-in simulation for candidate", "node", candidateName)

			// Run the simulation for this candidate against the current simView.
			if err = scaleInSim.Run(ctx, simView); err != nil {
				return sendPlanError(requestRef, fmt.Errorf("%w: failed for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err))
			}
			result, err := scaleInSim.Result()
			if err != nil {
				return sendPlanError(requestRef, fmt.Errorf("%w: failed to get result for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err))
			}

			if len(result.PodsToReschedule) == 0 {
				// Case A: All pods from scaled-in node were successfully rescheduled.
				log.V(3).Info("Scale-in simulation succeeded for candidate (all pods rescheduled)", "node", candidateName)
				scaledInSuccessNodes.Insert(candidateName)
				simView = result.View
				lastScaleInSuccessView = result.View
			} else {
				// There are pods that could not be rescheduled. Check if all of them have PDB disruptions allowed.
				// Case B: PodsToReschedule all have PodDisruptionBudget.Status.DisruptionsAllowed > 0
				// This is a placeholder — PDB checking logic is deferred until PDB tracking infra is available.
				// For now, treat any remaining pods as a scale-in failure for this candidate.
				log.V(3).Info("Scale-in simulation failed for candidate (pods remain unscheduled)",
					"node", candidateName, "podsToReschedule", len(result.PodsToReschedule))
				skipNodes.Insert(candidateName)
				simView = lastScaleInSuccessView
			}

			// Reset the simulation for the next candidate.
			if err = scaleInSim.Reset(); err != nil {
				return sendPlanError(requestRef, fmt.Errorf("failed to reset scale-in simulation: %w", err))
			}
		}
	}
}

// computePlanResult builds the [plannerapi.ScaleInPlanResult] from the set of successfully scaled-in nodes.
// A node is only included in the plan if it has been continuously identified as unneeded across invocations
// for at least the configured UnderutilizedDuration.
func (d *defaultSimulator) computePlanResult(log logr.Logger, memento *plannerapi.ScaleInMemento, scaledInSuccessNodes sets.Set[string], requestRef plannerapi.RequestRef) plannerapi.ScaleInPlanResult {
	now := time.Now()
	unneededDuration := d.simulatorConfig.UnderutilizedDuration

	// Ensure memento is initialized.
	if memento == nil {
		memento = &plannerapi.ScaleInMemento{}
	}
	if memento.LastIdentifiedUnneededNodes == nil {
		memento.LastIdentifiedUnneededNodes = make(map[string]time.Time)
	}

	// Update the memento with the currently identified unneeded nodes and determine which
	// nodes have exceeded the unneeded duration.
	var scaleInItems []sacorev1alpha1.ScaleInItem
	for nodeName := range scaledInSuccessNodes {
		firstSeen, exists := memento.LastIdentifiedUnneededNodes[nodeName]
		if !exists {
			// First time this node was identified as unneeded; record the timestamp but do not include in plan yet.
			log.V(3).Info("Node newly identified as unneeded, recording timestamp", "node", nodeName)
			memento.LastIdentifiedUnneededNodes[nodeName] = now
			continue
		}
		// The node was previously identified as unneeded. Check if the unneeded duration has been exceeded.
		if now.Sub(firstSeen) >= unneededDuration {
			log.V(2).Info("Node has exceeded unneeded duration, including in scale-in plan",
				"node", nodeName, "firstSeen", firstSeen, "unneededDuration", unneededDuration)
			scaleInItems = append(scaleInItems, sacorev1alpha1.ScaleInItem{
				NodeName: nodeName,
				// TODO: not sure how to populate nodePlacement
			})
		} else {
			log.V(3).Info("Node identified as unneeded but duration not yet exceeded",
				"node", nodeName, "firstSeen", firstSeen, "elapsed", now.Sub(firstSeen), "required", unneededDuration)
		}
	}

	// Prune memento entries for nodes that are no longer identified as unneeded.
	for nodeName := range memento.LastIdentifiedUnneededNodes {
		if _, stillUnneeded := scaledInSuccessNodes[nodeName]; !stillUnneeded {
			log.V(3).Info("Node no longer identified as unneeded, removing from memento", "node", nodeName)
			delete(memento.LastIdentifiedUnneededNodes, nodeName)
		}
	}

	planResult := plannerapi.ScaleInPlanResult{
		Memento: memento,
		Labels: map[string]string{
			commonconstants.LabelRequestID: requestRef.ID,
		},
	}
	if len(scaleInItems) > 0 {
		planResult.ScaleInPlan = &sacorev1alpha1.ScaleInPlan{
			Items: scaleInItems,
		}
	}
	return planResult
}

func nextCandidate(ctx context.Context, args plannerapi.ScaleInCandidateArgs) (*corev1.Node, error) {

}

// sendPlanError wraps the given error with the sentinel ErrGenScalingPlan and returns it as a ScaleInPlanResult.
func sendPlanError(requestRef plannerapi.RequestRef, err error) plannerapi.ScaleInPlanResult {
	return plannerapi.ScaleInPlanResult{
		Error: plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err),
	}
}

// New creates a new [plannerapi.ScaleInSimulator] that runs simulations for scale-in nodes.
func New(args plannerapi.SimulatorArgs, scaleInConfig plannerapi.ScaleInSimulatorConfig, candidateSelector plannerapi.ScaleInCandidateSelector) (plannerapi.ScaleInSimulator, error) {
	return &defaultSimulator{
		simulatorConfig:   scaleInConfig,
		viewAccess:        args.ViewAccess,
		schedulerLauncher: args.SchedulerLauncher,
		traceDir:          args.TraceDir,
	}, nil
}

// RequestState holds the internal Request scoped state of a ScaleInSimulator
type RequestState struct {
	viewAccess minkapi.ViewAccess
	// SimulationFactory is used to create `ScaleInSimulation`s
	SimulationFactory plannerapi.SimulationFactory
	// Request is the planner request being currently satisfied.
	Request  *plannerapi.Request
	ResultCh chan plannerapi.ScaleInPlanResult
	// SimRunCounter is a run counter for the number of simulation runs
	SimRunCounter *atomic.Uint32
	views         []minkapi.View
	simConfig     plannerapi.ScaleInSimulatorConfig
	mu            sync.Mutex
}

// RequestStateWith constructs a fresh simulator RequestState with the given planner Request and parameters
func RequestStateWith(request *plannerapi.Request, simConfig plannerapi.ScaleInSimulatorConfig,
	simulationFactory plannerapi.SimulationFactory, viewAccess minkapi.ViewAccess) RequestState {
	return RequestState{
		Request:           request,
		ResultCh:          make(chan plannerapi.ScaleInPlanResult),
		SimulationFactory: simulationFactory,
		SimRunCounter:     &atomic.Uint32{},
		simConfig:         simConfig,
		viewAccess:        viewAccess,
	}
}

// Reset clears and resets this RequestState
func (s *RequestState) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.views {
		if err := v.Close(); err != nil {
			// best-effort close
			_ = err
		}
	}
	clear(s.views)
	s.SimRunCounter.Store(0)
	s.Request = nil
	return nil
}
