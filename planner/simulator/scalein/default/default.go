package scaleinSimulator

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/planner/simulator/scalein"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ plannerapi.ScaleInSimulator = (*defaultSimulator)(nil)

type defaultSimulator struct {
	viewAccess             minkapi.ViewAccess
	schedulerLauncher      plannerapi.SchedulerLauncher
	traceDir               string
	state                  scalein.RequestState
	scaleInSimulatorConfig plannerapi.ScaleInSimulatorConfig
}

// New creates a new [plannerapi.ScaleInSimulator] that runs simulations for scale-in nodes.
func New(args plannerapi.SimulatorArgs, scaleInConfig plannerapi.ScaleInSimulatorConfig, candidateSelector plannerapi.ScaleInCandidateSelector) (plannerapi.ScaleInSimulator, error) {
	return &defaultSimulator{
		scaleInSimulatorConfig: scaleInConfig,
		viewAccess:             args.ViewAccess,
		schedulerLauncher:      args.SchedulerLauncher,
		traceDir:               args.TraceDir,
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
	d.state = scalein.RequestStateWith(request, d.scaleInSimulatorConfig, simulationFactory, d.viewAccess)
	go func() {
		defer close(d.state.ResultCh)
		if err := d.doSimulate(ctx); err != nil {
			scalein.SendPlanError(d.state.ResultCh, request.GetRef(), err)
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
		Name:              "scalein-sim",
		Config:            d.scaleInSimulatorConfig,
	}
	scaleInSim, err := d.state.SimulationFactory.NewScaleIn(simArgs)
	if err != nil {
		return fmt.Errorf("%w: %w", plannerapi.ErrCreateSimulation, err)
	}

	// Initialize views and loop state.
	simView := d.state.RequestView()
	scaledInSuccessNodes := sets.New[string]()
	skipNodes := sets.New[string]()

	// Begin candidate selection loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Select the next scale-in candidate.
			nextCandidate, err := getNextCandidate(ctx, plannerapi.ScaleInCandidateArgs{
				Constraint: d.state.Request.Constraint.Spec, // FIXME: pass actual constraint
				View:       simView,
				RequestRef: d.state.Request.GetRef(),
				SkipNodes:  skipNodes,
			})
			if err != nil {
				return fmt.Errorf("failed to select scale-in candidate: %w", err)
			}
			// If no candidate is returned, we have exhausted all options and can exit the loop to compute the final plan.
			if nextCandidate == nil {
				log.V(3).Info("No more scale-in candidates available, ending simulation loop.")
				// Compute ScaleInItems.
				scaleInItems := d.computeScaleInItems(ctx, scaledInSuccessNodes)
				scalein.SendPlanResult(d.state.Request, d.state.ResultCh, scaleInItems)
			}

			candidateName := nextCandidate.Name
			log.V(3).Info("Running scale-in simulation for candidate", "node", candidateName)

			// Run the simulation for this candidate against the current simView.
			if err = scaleInSim.Run(ctx, simView); err != nil {
				return fmt.Errorf("%w: failed for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}
			result, err := scaleInSim.Result()
			if err != nil {
				return fmt.Errorf("%w: failed to get result for node %q: %w", plannerapi.ErrRunSimulation, candidateName, err)
			}

			// If the simulation result contains zero pods to reschedule, we consider the scale-in successful for this candidate
			// and add it to the set of scaled-in nodes.
			if len(result.PodsToReschedule) == 0 {
				// Case A: All pods from scaled-in node were successfully rescheduled.
				log.V(3).Info("Scale-in simulation succeeded for candidate (all pods rescheduled)", "node", candidateName)
				scaledInSuccessNodes.Insert(candidateName)
				simView = result.View
			} else {
				// There are pods that could not be rescheduled. Check if all of them have PDB disruptions allowed.
				// Case B: PodsToReschedule all have PodDisruptionBudget.Status.DisruptionsAllowed > 0
				// This is a placeholder — PDB checking logic is deferred until PDB tracking infra is available.
				// For now, treat any remaining pods as a scale-in failure for this candidate.
				log.V(3).Info("Scale-in simulation failed for candidate (pods remain unscheduled)",
					"node", candidateName, "podsToReschedule", len(result.PodsToReschedule))
				skipNodes.Insert(candidateName)
			}
		}
	}
}

// getNextCandidate selects the next scale-in candidate node from the view based on the selection criteria:
// - Not in skipNodes
// - NodePool min count not reached
// - Not annotated with scale-in disabled
// - Does not have any pods with SafeToEvict=false
// - (TODO) Does not have pods that would violate PDBs
// - (TODO) Meets utilization threshold requirements
// Among the candidates that meet the criteria, those with the lowest priority (based on pool and template priority)
// are selected, and one is randomly returned from that set.
func getNextCandidate(ctx context.Context, args plannerapi.ScaleInCandidateArgs) (*corev1.Node, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Get all nodes from the view.
	nodes, err := args.View.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	// Build a pool name -> NodePool lookup from the constraint.
	poolByName := make(map[string]sacorev1alpha1.NodePool, len(args.Constraint.NodePools))
	for _, pool := range args.Constraint.NodePools {
		poolByName[pool.Name] = pool
	}

	// Count nodes per pool (before any filtering) for the Min check.
	nodesPerPool := make(map[string]int32)
	for i := range nodes {
		poolName := nodes[i].Labels[commonconstants.LabelNodePoolName]
		nodesPerPool[poolName]++
	}

	// Get all pods for SafeToEvict check.
	allPods, err := args.View.ListPods(ctx, minkapi.MatchAllCriteria)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	// Build a map of node name -> pods assigned to that node.
	podsByNode := make(map[string][]corev1.Pod)
	for i := range allPods {
		nodeName := allPods[i].Spec.NodeName
		if nodeName != "" {
			podsByNode[nodeName] = append(podsByNode[nodeName], allPods[i])
		}
	}

	// Filter candidates.
	var candidates []corev1.Node
	for i, node := range nodes {
		nodeName := node.Name
		poolName := node.Labels[commonconstants.LabelNodePoolName]

		// Skip if in skipNodes.
		if args.SkipNodes.Has(nodeName) {
			continue
		}

		// Skip if NodePool.Min has been reached.
		if pool, ok := poolByName[poolName]; ok && pool.Min > 0 {
			if nodesPerPool[poolName] <= pool.Min {
				log.V(5).Info("Skipping node: pool has reached minimum node count",
					"node", nodeName, "pool", poolName, "min", pool.Min, "current", nodesPerPool[poolName])
				continue
			}
		}

		// Skip if node is marked with ScaleInDisabledKey.
		if _, disabled := node.Annotations[commonconstants.AnnotationScaleInDisabledKey]; disabled {
			log.V(5).Info("Skipping node: scale-in disabled via annotation", "node", nodeName)
			continue
		}

		// Skip if node has pods with SafeToEvict = "false".
		if hasNonEvictablePod(podsByNode[nodeName]) {
			log.V(5).Info("Skipping node: has pods with safe-to-evict=false", "node", nodeName)
			continue
		}

		// TODO: Skip nodes with pods that have disrupted PodDisruptionBudgets (requires PDBTracker implementation).

		// TODO: Check node utilization against UtilizationThresholds via NodeUtilizationCalculator.

		candidates = append(candidates, nodes[i])
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Sort candidates by increasing priority (lowest priority first — those are the best scale-in candidates).
	slices.SortFunc(candidates, func(a, b corev1.Node) int {
		pkA := getNodePriorityKey(a, poolByName)
		pkB := getNodePriorityKey(b, poolByName)
		// CmpPriorityKeyDecreasing sorts high-priority first. We negate to get increasing order.
		return -commontypes.CmpPriorityKeyDecreasing(pkA, pkB)
	})

	// Get the sublist of candidates at the lowest priority.
	lowestPK := getNodePriorityKey(candidates[0], poolByName)
	var lowestPriorityCandidates []corev1.Node
	for i := range candidates {
		if getNodePriorityKey(candidates[i], poolByName) != lowestPK {
			break
		}
		lowestPriorityCandidates = append(lowestPriorityCandidates, candidates[i])
	}

	// Randomize the lowest priority candidates.
	// #nosec G404 -- cryptographic randomness not required here
	rand.Shuffle(len(lowestPriorityCandidates), func(i, j int) {
		lowestPriorityCandidates[i], lowestPriorityCandidates[j] = lowestPriorityCandidates[j], lowestPriorityCandidates[i]
	})

	return &lowestPriorityCandidates[0], nil
}

// computeScaleInItems builds the list of scale-in items from the set of successfully scaled-in nodes.
// A node is only included in the plan if it has been continuously identified as unneeded across invocations
// for at least the configured UnderutilizedDuration.
func (d *defaultSimulator) computeScaleInItems(ctx context.Context, scaledInSuccessNodes sets.Set[string]) (scaleInItems []sacorev1alpha1.ScaleInItem) {
	log := logr.FromContextOrDiscard(ctx)
	now := time.Now()
	unneededDuration := d.scaleInSimulatorConfig.UnderutilizedDuration

	// Ensure memento is initialized.
	if d.state.Request == nil {
		d.state.Request = &plannerapi.Request{}
	}
	if d.state.Request.Memento == nil {
		d.state.Request.Memento = &plannerapi.Memento{}
	}
	if d.state.Request.Memento.ScaleIn == nil {
		d.state.Request.Memento.ScaleIn = &plannerapi.ScaleInMemento{}
	}
	if d.state.Request.Memento.ScaleIn.LastIdentifiedUnneededNodes == nil {
		d.state.Request.Memento.ScaleIn.LastIdentifiedUnneededNodes = make(map[string]time.Time)
	}

	// Update the memento with the currently identified unneeded nodes and determine which
	// nodes have exceeded the unneeded duration.
	for nodeName := range scaledInSuccessNodes {
		firstSeen, exists := d.state.Request.Memento.ScaleIn.LastIdentifiedUnneededNodes[nodeName]
		if !exists {
			// First time this node was identified as unneeded; record the timestamp but do not include in plan yet.
			log.V(3).Info("Node newly identified as unneeded, recording timestamp", "node", nodeName)
			d.state.Request.Memento.ScaleIn.LastIdentifiedUnneededNodes[nodeName] = now
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
			// can there be any failure case where we would want to keep this node in the memento for consideration in the next plan generation loop?
			delete(d.state.Request.Memento.ScaleIn.LastIdentifiedUnneededNodes, nodeName)
		} else {
			log.V(3).Info("Node identified as unneeded but duration not yet exceeded",
				"node", nodeName, "firstSeen", firstSeen, "elapsed", now.Sub(firstSeen), "required", unneededDuration)
		}
	}

	return
}

// hasNonEvictablePod returns true if any pod in the slice has the SafeToEvict annotation set to "false".
func hasNonEvictablePod(pods []corev1.Pod) bool {
	for i := range pods {
		if val, ok := pods[i].Annotations[commonconstants.AnnotationSafeToEvict]; ok && val == "false" {
			return true
		}
	}
	return false
}

// getNodePriorityKey returns the PriorityKey for a node by looking up its pool and template in the constraint.
func getNodePriorityKey(node corev1.Node, poolByName map[string]sacorev1alpha1.NodePool) commontypes.PriorityKey {
	poolName := node.Labels[commonconstants.LabelNodePoolName]
	pool, ok := poolByName[poolName]
	if !ok {
		return commontypes.PriorityKey{}
	}
	templateName := node.Labels[commonconstants.LabelNodeTemplateName]
	for _, nt := range pool.NodeTemplates {
		if nt.Name == templateName {
			return commontypes.PriorityKey{First: pool.Priority, Second: nt.Priority}
		}
	}
	return commontypes.PriorityKey{First: pool.Priority}
}
