package scaleinsimulator

import (
	"context"
	"fmt"
	"time"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	pdbtracker "github.com/gardener/scaling-advisor/planner/pdbtracker"
	scalein "github.com/gardener/scaling-advisor/planner/simulator/scalein"
	"github.com/go-logr/logr"
)

var _ plannerapi.ScaleInSimulator = (*scaleInSimulator)(nil)

type scaleInSimulator struct {
	viewAccess               minkapi.ViewAccess
	schedulerLauncher        plannerapi.SchedulerLauncher
	traceDir                 string
	state                    scalein.SimulatorState
	scaleInCandidateSelector plannerapi.ScaleInCandidateSelector
	simulatorConfig          plannerapi.SimulatorConfig
	simulationFactory        plannerapi.SimulationFactory
	pdbTracker               plannerapi.PDBTracker
}

// New creates a new [plannerapi.ScaleInSimulator] that runs simulations for scale-in nodes.
func New(args plannerapi.SimulatorArgs) (plannerapi.ScaleInSimulator, error) {
	return &scaleInSimulator{
		simulatorConfig:          args.Config,
		scaleInCandidateSelector: args.ScaleInCandidateSelector,
		viewAccess:               args.ViewAccess,
		schedulerLauncher:        args.SchedulerLauncher,
		traceDir:                 args.TraceDir,
		simulationFactory:        args.SimulationFactory,
		pdbTracker:               pdbtracker.New(),
	}, nil
}

func (d *scaleInSimulator) Close() error {
	return d.state.Reset()
}

// Simulate implements [plannerapi.ScaleInSimulator.Simulate]. It iteratively selects scale-in candidates,
// runs a simulation for each candidate, and accumulates successfully scaled-in nodes. A node is only included
// in the final [plannerapi.ScaleInPlanResult] if it has been continuously identified as unneeded across invocations
// for at least the configured UnderutilizedDuration (tracked via the [plannerapi.ScaleInMemento]).
func (d *scaleInSimulator) Simulate(ctx context.Context, request *plannerapi.Request) <-chan plannerapi.ScaleInPlanResult {
	d.state = scalein.NewSimulatorState(request, d.simulatorConfig, d.simulationFactory, d.viewAccess)
	go func() {
		defer close(d.state.ResultCh)
		if err := d.doSimulate(ctx); err != nil {
			scalein.SendPlanError(request.GetRef(), d.state.ResultCh, d.state.Request.Memento.ScaleIn, err)
		}
	}()
	return d.state.ResultCh
}

// doSimulate contains the main simulation loop for the default scale-in simulator. It is responsible for selecting candidates,
// running simulations, and accumulating results to produce the final scale-in plan.
func (d *scaleInSimulator) doSimulate(ctx context.Context) (err error) {
	log := logr.FromContextOrDiscard(ctx)

	// TODO: move the initializeRequestView out to the planner, same is used for scale-out and scale-in.
	if err = d.state.InitializeRequestView(ctx); err != nil {
		return
	}

	// Create the scale-in simulation.
	simArgs := plannerapi.ScaleInSimArgs{
		SchedulerLauncher: d.schedulerLauncher,
		RunCounter:        d.state.SimRunCounter,
		//TODO: Name might not be required -> add requestId to the end.
		Name:     "scalein-sim",
		TraceDir: d.traceDir,
		Config:   d.simulatorConfig,
	}
	scaleInSim, err := d.state.SimulationFactory.NewScaleIn(simArgs)
	if err != nil {
		return fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulation, err)
	}

	// Initialize views and loop state.
	simView := d.state.RequestView()
	scaleInNomineeNodes := map[string]sacorev1alpha1.ScaleInItem{}
	memento := d.state.Request.Memento.ScaleIn

	if err := d.scaleInCandidateSelector.Init(ctx, plannerapi.ScaleInCandidateSelectorArgs{
		Constraint: d.state.Request.Constraint.Spec,
		View:       simView,
	}); err != nil {
		return err
	}

	// Set PDBs in tracker from the snapshot
	if err := d.pdbTracker.SetPDBs(d.state.Request.Snapshot.PDBs); err != nil {
		return err
	}

	// Begin candidate selection loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Select the next scale-in candidate.
			nextCandidate, err := d.scaleInCandidateSelector.NextCandidate(ctx, plannerapi.ScaleInCandidateSelectorArgs{
				Constraint:            d.state.Request.Constraint.Spec,
				View:                  simView,
				PDBTracker:            d.pdbTracker,
				UtilizationThresholds: d.simulatorConfig.UtilizationThresholds,
			})
			if err != nil {
				return fmt.Errorf("failed to select scale-in candidate: %w", err)
			}
			// If no candidate is returned, we have exhausted all options and can exit the loop to compute the final plan.
			if nextCandidate == nil {
				log.V(3).Info("No more scale-in candidates available, ending simulation loop.")
				// Compute ScaleInItems.
				scaleInItems, err := d.computeScaleInItems(ctx, &memento, scaleInNomineeNodes)
				if err != nil {
					return fmt.Errorf("failed to compute scale-in items: %w", err)
				}
				if len(scaleInItems) == 0 {
					scalein.SendPlanError(d.state.Request.GetRef(), d.state.ResultCh, memento, plannerapi.ErrNoScaleInPlan)
				} else {
					scalein.SendPlanResult(d.state.Request.GetRef(), d.state.ResultCh, memento, scaleInItems)
				}
				return nil
			}

			candidateName := nextCandidate.Name
			log.V(3).Info("Running scale-in simulation for candidate", "node", candidateName)

			// Run the simulation for this candidate against the current simView.
			if err = scaleInSim.Run(ctx, simView, nextCandidate); err != nil {
				return fmt.Errorf("%w: failed for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}
			result, err := scaleInSim.Result()
			if err != nil {
				return fmt.Errorf("%w: failed to get result for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}

			// All pods displaced from the scale-in node were successfully rescheduled
			// without making any previously schedulable pod unschedulable
			if result.IsSimulationSuccess {
				// All pods from scaled-in node were successfully rescheduled.
				log.V(3).Info("Scale-in simulation succeeded for candidate (all pods rescheduled)", "node", candidateName)
				scaleInNomineeNodes[candidateName] = result.Item
				simView = result.View
			} else {
				// There are pods that could not be rescheduled. We treat this as scale-in failure.
				log.V(3).Info("Scale-in simulation failed for candidate",
					"node", candidateName)
			}
			d.scaleInCandidateSelector.RemoveCandidateNode(candidateName)
		}
	}
}

// computeScaleInItems builds the list of scale-in items from the set of successfully scaled-in nodes.
// A node is only included in the plan if it has been continuously identified as unneeded across [plannerapi.ScalingPlanner.Plan] invocations
// for at least the configured UnderutilizedDuration.
func (d *scaleInSimulator) computeScaleInItems(ctx context.Context, memento *plannerapi.ScaleInMemento, scaledInSuccessNodes map[string]sacorev1alpha1.ScaleInItem) ([]sacorev1alpha1.ScaleInItem, error) {
	log := logr.FromContextOrDiscard(ctx)
	now := time.Now()
	unneededDuration := d.simulatorConfig.UnderutilizedDuration

	// Ensure memento is initialized.
	if memento.LastIdentifiedUnneededNodes == nil {
		memento.LastIdentifiedUnneededNodes = make(map[string]time.Time)
	}

	// Update the memento with the currently identified unneeded nodes and determine which
	// nodes have exceeded the unneeded duration.
	var scaleInItems []sacorev1alpha1.ScaleInItem
	for nodeName, scaleInItem := range scaledInSuccessNodes {
		firstSeenAt, exists := memento.LastIdentifiedUnneededNodes[nodeName]
		if !exists {
			// First time this node was identified as unneeded; record the timestamp but do not include in plan yet.
			log.V(3).Info("Node newly identified as unneeded, recording timestamp", "node", nodeName)
			memento.LastIdentifiedUnneededNodes[nodeName] = now
			continue
		}
		// The node was previously identified as unneeded. Check if the unneeded duration has been exceeded.
		if now.Sub(firstSeenAt) >= unneededDuration {
			log.V(2).Info("Node has exceeded unneeded duration, including in scale-in plan",
				"node", nodeName, "firstSeen", firstSeenAt, "unneededDuration", unneededDuration)
			scaleInItems = append(scaleInItems, scaleInItem)
		} else {
			log.V(3).Info("Node identified as unneeded but duration not yet exceeded",
				"node", nodeName, "firstSeen", firstSeenAt, "elapsed", now.Sub(firstSeenAt), "required", unneededDuration)
		}
	}

	return scaleInItems, nil
}
