package scaleinsimulator

import (
	"context"
	"fmt"
	"time"

	scalein "github.com/gardener/scaling-advisor/planner/simulator/scalein"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/go-logr/logr"
)

var _ plannerapi.ScaleInSimulator = (*scaleInSimulator)(nil)

type scaleInSimulator struct {
	viewAccess               minkapi.ViewAccess
	schedulerLauncher        plannerapi.SchedulerLauncher
	scaleInCandidateSelector plannerapi.ScaleInCandidateSelector
	simulationFactory        plannerapi.SimulationFactory
	traceDir                 string
	simulatorConfig          plannerapi.SimulatorConfig
	state                    scalein.SimulatorState
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
	}, nil
}

func (d *scaleInSimulator) Close() error {
	return d.state.Reset()
}

// Simulate implements [plannerapi.ScaleInSimulator.Simulate]. It iteratively selects scale-in candidates,
// runs a simulation for each candidate, and accumulates successfully scaled-in nodes. A node is only included
// in the final [plannerapi.ScaleInPlanResult] if it has been continuously identified as unneeded across invocations
// for at least the configured UnderutilizedDuration (tracked via the [plannerapi.ScaleInMemento]).
func (d *scaleInSimulator) Simulate(ctx context.Context, request *plannerapi.Request, requestView minkapi.View) <-chan plannerapi.ScaleInPlanResult {
	d.state = scalein.NewSimulatorState(request, d.simulatorConfig, d.simulationFactory, d.viewAccess, requestView)
	go func() {
		defer close(d.state.ResultCh)
		if err := d.doSimulate(ctx); err != nil {
			scalein.SendPlanError(request.GetRef(), d.state.ResultCh, d.state.RequestView(), d.state.Request.Memento.ScaleIn, err)
		}
	}()
	return d.state.ResultCh
}

// doSimulate contains the main simulation loop for the default scale-in simulator. It is responsible for selecting candidates,
// running simulations, and accumulating results to produce the final scale-in plan.
func (d *scaleInSimulator) doSimulate(ctx context.Context) (err error) {
	log := logr.FromContextOrDiscard(ctx)

	// Create the scale-in simulation.
	simArgs := plannerapi.ScaleInSimArgs{
		SchedulerLauncher: d.schedulerLauncher,
		RunCounter:        d.state.SimRunCounter,
		Name:              objutil.GenerateName("scalein-sim"),
		TraceDir:          d.traceDir,
		Config:            d.simulatorConfig,
	}
	scaleInSim, err := d.state.SimulationFactory.NewScaleIn(simArgs)
	if err != nil {
		return fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulation, err)
	}

	// Initialize candidate selector
	err = d.scaleInCandidateSelector.Init(ctx, plannerapi.ScaleInCandidateSelectorArgs{
		Constraint: d.state.Request.Constraint.Spec,
		View:       d.state.RequestView(),
	})
	if err != nil {
		return err
	}

	// Set PDBs in tracker from the snapshot
	err = d.state.PdbTracker.SetPDBs(d.state.Request.Snapshot.PDBs)
	if err != nil {
		return err
	}

	// Begin candidate selection loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Select the next scale-in candidate.
			nextCandidate, err := d.scaleInCandidateSelector.Next(ctx, plannerapi.ScaleInCandidateSelectorArgs{
				Constraint:            d.state.Request.Constraint.Spec,
				View:                  d.state.RequestView(),
				PDBTracker:            d.state.PdbTracker,
				UtilizationThresholds: d.simulatorConfig.UtilizationThresholds,
			})
			if err != nil {
				return fmt.Errorf("failed to select scale-in candidate: %w", err)
			}
			// If no candidate is returned, we have exhausted all options and can exit the loop to compute the final plan.
			if nextCandidate == nil {
				log.V(3).Info("No more scale-in candidates available, ending simulation loop.")
				// Compute ScaleInItems.
				scaleInItems := d.computeScaleInItems(ctx)
				if len(scaleInItems) == 0 {
					scalein.SendPlanError(d.state.Request.GetRef(), d.state.ResultCh, d.state.RequestView(), d.state.Memento, plannerapi.ErrNoScaleInPlan)
				} else {
					scalein.SendPlanResult(d.state.Request.GetRef(), d.state.ResultCh, d.state.RequestView(), d.state.Memento, scaleInItems)
				}
				return nil
			}

			candidateName := nextCandidate.Name
			log.V(3).Info("Running scale-in simulation for candidate", "node", candidateName)

			// Run the simulation for this candidate against the current simView. The
			// GetViewFunc here returns the shared request view; once per-candidate sandboxing
			// is wired up, this should switch to d.state.CreateSandboxView(ctx, name, d.state.RequestView()).
			getViewFn := func(_ context.Context, _ string) (minkapi.View, error) {
				return d.state.RequestView(), nil
			}
			if err = scaleInSim.Run(ctx, getViewFn, nextCandidate); err != nil {
				return fmt.Errorf("%w: failed for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}
			result, err := scaleInSim.Result()
			if err != nil {
				return fmt.Errorf("%w: failed to get result for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}

			// All pods displaced from the scale-in node were successfully rescheduled
			// without making any previously schedulable pod unschedulable
			if result.IsSimulationSuccess {
				log.V(3).Info("Scale-in simulation succeeded for candidate (all pods safely rescheduled)", "node", candidateName)
				d.state.ScaleInNomineeNodes[candidateName] = result.Item
				d.state.SetRequestView(result.View)
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
func (d *scaleInSimulator) computeScaleInItems(ctx context.Context) []sacorev1alpha1.ScaleInItem {
	log := logr.FromContextOrDiscard(ctx)
	now := time.Now()
	unneededDuration := d.simulatorConfig.UnderutilizedDuration
	memento := &d.state.Memento

	// Ensure memento is initialized.
	if memento.LastIdentifiedUnneededNodes == nil {
		memento.LastIdentifiedUnneededNodes = make(map[string]time.Time)
	}

	var scaleInItems []sacorev1alpha1.ScaleInItem
	for nodeName, scaleInItem := range d.state.ScaleInNomineeNodes {
		firstSeenAt, exists := memento.LastIdentifiedUnneededNodes[nodeName]
		if !exists {
			log.V(3).Info("Node newly identified as unneeded, recording timestamp", "node", nodeName)
			memento.LastIdentifiedUnneededNodes[nodeName] = now
			continue
		}
		if now.Sub(firstSeenAt) >= unneededDuration {
			log.V(2).Info("Node has exceeded unneeded duration, including in scale-in plan",
				"node", nodeName, "firstSeen", firstSeenAt, "unneededDuration", unneededDuration)
			scaleInItems = append(scaleInItems, scaleInItem)
		} else {
			log.V(3).Info("Node identified as unneeded but duration not yet exceeded",
				"node", nodeName, "firstSeen", firstSeenAt, "elapsed", now.Sub(firstSeenAt), "required", unneededDuration)
		}
	}

	return scaleInItems
}
