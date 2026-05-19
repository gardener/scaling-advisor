package defaultSimulator

import (
	"context"
	"fmt"
	"time"

	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/planner/pdb"
	defaultpdb "github.com/gardener/scaling-advisor/planner/pdb/defaultpdb"
	"github.com/gardener/scaling-advisor/planner/simulator/scalein"
	"github.com/go-logr/logr"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ plannerapi.ScaleInSimulator = (*defaultSimulator)(nil)

type defaultSimulator struct {
	viewAccess               minkapi.ViewAccess
	schedulerLauncher        plannerapi.SchedulerLauncher
	traceDir                 string
	state                    scalein.SimulatorState
	scaleInCandidateSelector plannerapi.ScaleInCandidateSelector
	scaleInSimulatorConfig   plannerapi.ScaleInSimulatorConfig
}

// New creates a new [plannerapi.ScaleInSimulator] that runs simulations for scale-in nodes.
func New(args plannerapi.SimulatorArgs) (plannerapi.ScaleInSimulator, error) {
	return &defaultSimulator{
		scaleInSimulatorConfig:   args.ScaleInSimulatorConfig,
		scaleInCandidateSelector: args.ScaleInCandidateSelector,
		viewAccess:               args.ViewAccess,
		schedulerLauncher:        args.SchedulerLauncher,
		traceDir:                 args.TraceDir,
	}, nil
}

func (d *defaultSimulator) Close() error {
	return d.state.Reset()
}

// Simulate implements [plannerapi.ScaleInSimulator.Simulate]. It iteratively selects scale-in candidates,
// runs a simulation for each candidate, and accumulates successfully scaled-in nodes. A node is only included
// in the final [plannerapi.ScaleInPlanResult] if it has been continuously identified as unneeded across invocations
// for at least the configured UnderutilizedDuration (tracked via the [plannerapi.ScaleInMemento]).
func (d *defaultSimulator) Simulate(ctx context.Context, request *plannerapi.Request, simulationFactory plannerapi.SimulationFactory) <-chan plannerapi.ScaleInPlanResult {
	d.state = scalein.NewSimulatorState(request, d.scaleInSimulatorConfig, simulationFactory, d.viewAccess)
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
func (d *defaultSimulator) doSimulate(ctx context.Context) (err error) {
	log := logr.FromContextOrDiscard(ctx)

	if err = d.state.InitializeRequestView(ctx); err != nil {
		return
	}

	// Create the scale-in simulation.
	simArgs := plannerapi.ScaleInSimArgs{
		SchedulerLauncher: d.schedulerLauncher,
		RunCounter:        d.state.SimRunCounter,
		//TODO: Name might not be required
		Name:     "scalein-sim",
		TraceDir: d.traceDir,
		Config:   d.scaleInSimulatorConfig,
	}
	scaleInSim, err := d.state.SimulationFactory.NewScaleIn(simArgs)
	if err != nil {
		return fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulation, err)
	}

	// Initialize views and loop state.
	simView := d.state.RequestView()
	//TODO: Take a look at scaleInSuccessNodes name. Find a better name. -> scaleInNomineeNodes
	scaledInSuccessNodes := map[string]sacorev1alpha1.ScaleInItem{}
	skipNodes := sets.New[string]()
	memento := d.state.Request.Memento.ScaleIn

	// Initialize PDB tracker.
	pdbTracker, err := initPdbTracker(d.state.Request.Snapshot.PDBs)
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
			nextCandidate, err := d.scaleInCandidateSelector.NextCandidate(ctx, plannerapi.ScaleInCandidateArgs{
				Constraint: d.state.Request.Constraint.Spec, // FIXME: pass actual constraint
				View:       simView,
				RequestRef: d.state.Request.GetRef(),
				PDBTracker: pdbTracker,
				//TODO: No need to pass pointer to set. Just set should be fine.
			}, &skipNodes)
			if err != nil {
				return fmt.Errorf("failed to select scale-in candidate: %w", err)
			}
			// If no candidate is returned, we have exhausted all options and can exit the loop to compute the final plan.
			if nextCandidate == nil {
				log.V(3).Info("No more scale-in candidates available, ending simulation loop.")
				// Compute ScaleInItems.
				scaleInItems, err := d.computeScaleInItems(ctx, &memento, scaledInSuccessNodes)
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
			if err = scaleInSim.Run(ctx, simView, candidateName); err != nil {
				return fmt.Errorf("%w: failed for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}
			result, err := scaleInSim.Result()
			if err != nil {
				return fmt.Errorf("%w: failed to get result for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}

			// // If the simulation result contains zero pods to reschedule, we consider the scale-in successful for this candidate
			// // and add it to the set of scaled-in nodes.
			if len(result.PodsToReschedule) == 0 {
				// Case A: All pods from scaled-in node were successfully rescheduled.
				log.V(3).Info("Scale-in simulation succeeded for candidate (all pods rescheduled)", "node", candidateName)
				scaledInSuccessNodes[candidateName] = result.Items[0]
				simView = result.View
				// // TODO: reduce the disruptionsAllowed for pods with PDBs defined and were evicted.
				// // Do I also reduce the pdbs at the start of the simulation loop to account for all the unscheduled pods present before scaling in?
				// if len(result.PodsToReschedule) > 0 {
				// 	podToReschedule, ok := result.PodsToReschedule.PopAny()
				// 	if !ok {
				// 		return fmt.Errorf("failed to pop pod from PodsToReschedule")
				// 	}
				// 	podsMatchingCriteria := minkapi.MatchCriteria{
				// 		Namespace: podToReschedule.Namespace,
				// 	}
				// 	for podName := range result.PodsToReschedule {
				// 		podsMatchingCriteria.Names.Insert(podName.Name)
				// 	}
				// 	pods, err := simView.ListPods(ctx, podsMatchingCriteria)
				// 	if err != nil {
				// 		return fmt.Errorf("failed to list pods matching criteria: %w", err)
				// 	}
				// 	podsPtr := make([]*v1.Pod, len(pods))
				// 	for i := range pods {
				// 		podsPtr[i] = &pods[i]
				// 	}
				// 	pdbTracker.RemovePods(podsPtr)
				// }
			} else {
				// There are pods that could not be rescheduled. Check if all of them have PDB disruptions allowed.
				// Case B: PodsToReschedule all have PodDisruptionBudget.Status.DisruptionsAllowed > 0
				// This is a placeholder — PDB checking logic is deferred until PDB tracking infra is available.
				// For now, treat any remaining pods as a scale-in failure for this candidate.
				log.V(3).Info("Scale-in simulation failed for candidate (pods remain unscheduled)",
					"node", candidateName, "podsToReschedule", len(result.PodsToReschedule))
				skipNodes.Insert(candidateName)
			}
			// result.Items is expected to contain exactly one item for the candidate node if the simulation was successful for that node, and be empty if the simulation failed to scale in that node.
			// scaledInSuccessNodes[candidateName] = result.Items[0]
		}
	}
}

// computeScaleInItems builds the list of scale-in items from the set of successfully scaled-in nodes.
// A node is only included in the plan if it has been continuously identified as unneeded across invocations
// for at least the configured UnderutilizedDuration.
func (d *defaultSimulator) computeScaleInItems(ctx context.Context, memento *plannerapi.ScaleInMemento, scaledInSuccessNodes map[string]sacorev1alpha1.ScaleInItem) ([]sacorev1alpha1.ScaleInItem, error) {
	log := logr.FromContextOrDiscard(ctx)
	now := time.Now()
	unneededDuration := d.scaleInSimulatorConfig.UnderutilizedDuration

	// Ensure memento is initialized.
	if memento.LastIdentifiedUnneededNodes == nil {
		memento.LastIdentifiedUnneededNodes = make(map[string]time.Time)
	}

	// Update the memento with the currently identified unneeded nodes and determine which
	// nodes have exceeded the unneeded duration.
	var scaleInItems []sacorev1alpha1.ScaleInItem
	for nodeName, scaleInItem := range scaledInSuccessNodes {
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
			scaleInItems = append(scaleInItems, scaleInItem)
			// can there be any failure case where we would want to keep this node in the memento for consideration in the next plan generation loop?
			delete(memento.LastIdentifiedUnneededNodes, nodeName)
		} else {
			log.V(3).Info("Node identified as unneeded but duration not yet exceeded",
				"node", nodeName, "firstSeen", firstSeen, "elapsed", now.Sub(firstSeen), "required", unneededDuration)
		}
	}

	return scaleInItems, nil
}

// initPdbTracker creates a RemainingPdbTracker and populates it with PDBs from the given ClusterSnapshot.
func initPdbTracker(snapshotPDBs []policyv1.PodDisruptionBudget) (pdb.RemainingPdbTracker, error) {
	pdbPtrs := make([]*policyv1.PodDisruptionBudget, len(snapshotPDBs))
	for i := range snapshotPDBs {
		pdbPtrs[i] = &snapshotPDBs[i]
	}
	tracker := defaultpdb.NewDefaultRemainingPdbTracker()
	if err := tracker.SetPdbs(pdbPtrs); err != nil {
		return nil, fmt.Errorf("failed to set PDBs on tracker: %w", err)
	}
	return tracker, nil
}
