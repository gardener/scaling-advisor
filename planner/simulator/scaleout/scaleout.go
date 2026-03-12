// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package scaleout provides common implementation types and helper routines used by ScaleOutSimulator implementations.
package scaleout

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gardener/scaling-advisor/planner/simulator"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/viewutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	"github.com/go-logr/logr"
)

var (
	_ commontypes.Resettable = (*SimulatorState)(nil)
)

// SimulatorState holds the internal state of a ScaleOutSimulator when executing simulations
type SimulatorState struct {
	viewAccess minkapi.ViewAccess
	// SimulationFactory is used to create `ScaleOutSimulation`s
	SimulationFactory plannerapi.SimulationFactory
	// Request is the planner request being currently satisfied.
	Request  *plannerapi.Request
	ResultCh chan plannerapi.ScaleOutPlanResult
	// SimRunCounter is a run counter for the number of simulation runs
	SimRunCounter *atomic.Uint32
	views         []minkapi.View
	// SimulationGroups is the slice of ScaleOutSimGroup (if any) created for satisfying the current request.
	SimulationGroups []plannerapi.ScaleOutSimGroup
	simConfig        plannerapi.SimulatorConfig
	mu               sync.Mutex
}

// NewSimulatorState constructs a fresh [SimulatorState] for the [plannerapi.ScaleOutSimulator] processing the given
// [plannerapi.Request] with the given parameters
func NewSimulatorState(request *plannerapi.Request, simConfig plannerapi.SimulatorConfig, simulationFactory plannerapi.SimulationFactory, viewAccess minkapi.ViewAccess) *SimulatorState {
	return &SimulatorState{
		Request:           request,
		ResultCh:          make(chan plannerapi.ScaleOutPlanResult),
		SimulationFactory: simulationFactory,
		SimRunCounter:     &atomic.Uint32{},
		simConfig:         simConfig,
		viewAccess:        viewAccess,
	}
}

// InitializeRequestView performs common initialization on this simulator state. This currently includes:
//   - populating the request view
//   - Binding volume claims for immediate volume binding mode
func (s *SimulatorState) InitializeRequestView(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	requestView, err := s.createRequestView(ctx, s.viewAccess)
	if err != nil {
		return err
	}

	if err = simulator.PopulateView(ctx, requestView, &s.Request.Snapshot); err != nil {
		err = fmt.Errorf("%w: %w", plannerapi.ErrPopulateRequestView, err)
		return err
	}

	if s.simConfig.BindVolumeClaimsForImmediateMode {
		// Run static PVC<->PV Binding for Immediate VolumeBinding mode. Can be done just once for in the requestView
		// for all simulations
		if _, err = volutil.BindClaimsForImmediateMode(ctx, requestView); err != nil {
			return err
		}
	}
	err = viewutil.LogObjects(ctx, "requestView", requestView)
	if err != nil {
		log.Info("failed to dump requestView objects", "requestView", requestView.GetName(), "error", err)
	}
	return nil
}

// CreateSandboxView creates a sandbox view with the given name from the given delegate view, adds the new view to
// internal slice of views and returns the same.
func (s *SimulatorState) CreateSandboxView(ctx context.Context, name string, delegate minkapi.View) (minkapi.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sandboxView, err := s.viewAccess.GetSandboxViewOverDelegate(ctx, name, delegate)
	if err != nil {
		return nil, err
	}
	s.views = append(s.views, sandboxView)
	return sandboxView, nil
}

// RequestView gets the request minkapi view within this state. request Views are views that only have the request
// cluster snapshot populated within them along with any initialization done by InitializeRequestView.
func (s *SimulatorState) RequestView() minkapi.View {
	if len(s.views) == 0 {
		return nil
	} else {
		return s.views[0]
	}
}

// Reset clears and resets this SimulatorState
func (s *SimulatorState) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, v := range s.views {
		if err := v.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	clear(s.views)
	s.SimRunCounter.Store(0)
	clear(s.SimulationGroups)
	s.Request = nil
	return errors.Join(errs...)
}

// SendPlanError wraps the given error within a sentinel error plannerapi.ErrGenScalingPlan, creates a ScaleOutPlanResult and
// sends the result on the planResultCh.
func SendPlanError(planResultCh chan<- plannerapi.ScaleOutPlanResult, requestRef plannerapi.RequestRef, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	planResultCh <- plannerapi.ScaleOutPlanResult{
		Error: err,
	}
}

// SendPlanResult creates a plannerapi.ScaleOutPlanResult from the given plannerapi.Request and plannerapi.SimulationGroupCycleResults
// and sends this result to the resultCh.
func SendPlanResult(ctx context.Context, resultCh chan<- plannerapi.ScaleOutPlanResult,
	req *plannerapi.Request, simulationRunCount uint32, // TODO: introduce a plannerapi.Metrics.
	groupCycleResults []plannerapi.ScaleOutSimGroupCycleResult) error {
	log := logr.FromContextOrDiscard(ctx)
	existingNodeCountByPlacement, err := req.Snapshot.GetNodeCountByPlacement()
	if err != nil {
		return err
	}
	planGenerateDuration := time.Since(req.CreationTime)
	numUnscheduledPods := len(req.Snapshot.GetUnscheduledPods())
	labels := map[string]string{
		commonconstants.LabelRequestID:                  req.ID,
		commonconstants.LabelCorrelationID:              req.CorrelationID,
		commonconstants.LabelTotalSimulationRuns:        fmt.Sprintf("%d", simulationRunCount),
		commonconstants.LabelPlanGenerateDuration:       planGenerateDuration.String(),
		commonconstants.LabelSnapshotNumUnscheduledPods: strconv.Itoa(numUnscheduledPods),
		commonconstants.LabelConstraintNumPools:         strconv.Itoa(len(req.Constraint.Spec.NodePools)),
	}
	var allWinnerNodeScores []plannerapi.NodeScore
	var leftOverUnscheduledPods []commontypes.NamespacedName
	for _, gcr := range groupCycleResults {
		allWinnerNodeScores = append(allWinnerNodeScores, gcr.WinnerNodeScores...)
		leftOverUnscheduledPods = gcr.LeftoverUnscheduledPods
	}
	scaleOutPlan := createScaleOutPlan(allWinnerNodeScores, existingNodeCountByPlacement, leftOverUnscheduledPods)
	planResult := plannerapi.ScaleOutPlanResult{
		Labels:       labels,
		ScaleOutPlan: &scaleOutPlan,
	}
	log.V(2).Info("Sent Planner Success Response", "response", planResult)
	resultCh <- planResult
	return nil
}

// CreateAllNodeTemplates creates a slice of all possible [plannerapi.ScaleOutNodeTemplate] for the given slice of
// [sacorev1alpha1.NodePool].
func CreateAllNodeTemplates(pools []sacorev1alpha1.NodePool) []plannerapi.ScaleOutNodeTemplate {
	allNodeTemplates := make([]plannerapi.ScaleOutNodeTemplate, 0, len(pools)*2)
	for _, np := range pools {
		for _, nt := range np.NodeTemplates {
			for _, az := range np.AvailabilityZones {
				allNodeTemplates = append(allNodeTemplates, createNodeTemplate(np, nt, az))
			}
		}
	}
	return allNodeTemplates
}

// GroupScaleOutNodeTemplatesByPriority does just exactly that and returns a map keyed by PriorityKey to slice of
// [plannerapi.ScaleOutNodeTemplate]
func GroupScaleOutNodeTemplatesByPriority(templates []plannerapi.ScaleOutNodeTemplate) map[commontypes.PriorityKey][]plannerapi.ScaleOutNodeTemplate {
	templatesByPriority := make(map[commontypes.PriorityKey][]plannerapi.ScaleOutNodeTemplate)
	for _, t := range templates {
		pk := t.PriorityKey
		group, ok := templatesByPriority[pk]
		if !ok {
			group = []plannerapi.ScaleOutNodeTemplate{t}
		}
		group = append(group, t)
		templatesByPriority[pk] = group
	}
	return templatesByPriority
}

// createNodeTemplate creates a [plannerapi.ScaleOutNodeTemplate] for the given [sacorev1alpha1.NodePool],
// [sacorev1alpha1.NodeTemplate] and availability zone.
func createNodeTemplate(pool sacorev1alpha1.NodePool, template sacorev1alpha1.NodeTemplate, zone string) plannerapi.ScaleOutNodeTemplate {
	return plannerapi.ScaleOutNodeTemplate{
		NodePlacement: sacorev1alpha1.NodePlacement{
			PoolName:         pool.Name,
			TemplateName:     template.Name,
			InstanceType:     template.InstanceType,
			Region:           pool.Region,
			AvailabilityZone: zone,
		},
		Labels:      pool.Labels,
		Annotations: pool.Annotations,
		Quota:       pool.Quota,
		Taints:      pool.Taints,
		PriorityKey: commontypes.PriorityKey{
			First:  pool.Priority,
			Second: template.Priority,
		},
		Capacity:       template.Capacity,
		KubeReserved:   template.KubeReserved,
		SystemReserved: template.SystemReserved,
		Architecture:   template.Architecture,
	}
}

func (s *SimulatorState) createRequestView(ctx context.Context, viewAccess minkapi.ViewAccess) (view minkapi.View, err error) {
	view, err = viewAccess.GetSandboxViewOverDelegate(ctx, "Request-"+s.Request.ID, viewAccess.GetBaseView())
	if err != nil {
		return
	}
	s.views = append(s.views, view)
	return
}

// createScaleOutPlan creates a ScaleOutPlan based on the given winningNodeScores, existingNodeCountByPlacement and leftoverUnscheduledPods.
func createScaleOutPlan(winningNodeScores []plannerapi.NodeScore, existingNodeCountByPlacement map[sacorev1alpha1.NodePlacement]int32, leftoverUnscheduledPods []commontypes.NamespacedName) sacorev1alpha1.ScaleOutPlan {
	scaleItems := make([]sacorev1alpha1.ScaleOutItem, 0, len(winningNodeScores))
	nodeScoresByPlacement := groupNodeScoresByNodePlacement(winningNodeScores)
	for placement, nodeScores := range nodeScoresByPlacement {
		delta := int32(len(nodeScores)) // #nosec G115 -- length of nodeScores cannot be greater than max int32.
		currentReplicas := existingNodeCountByPlacement[placement]
		scaleItems = append(scaleItems, sacorev1alpha1.ScaleOutItem{
			NodePlacement:   placement,
			CurrentReplicas: currentReplicas,
			Delta:           delta,
		})
	}
	return sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: objutil.GetFullNames(leftoverUnscheduledPods),
		Items:               scaleItems,
	}
}

// groupNodeScoresByNodePlacement groups the given nodeScores by their NodePlacement and returns a map of NodePlacement to slice of NodeScores.
func groupNodeScoresByNodePlacement(nodeScores []plannerapi.NodeScore) map[sacorev1alpha1.NodePlacement][]plannerapi.NodeScore {
	groupByPlacement := make(map[sacorev1alpha1.NodePlacement][]plannerapi.NodeScore)
	for _, ns := range nodeScores {
		groupByPlacement[ns.Placement] = append(groupByPlacement[ns.Placement], ns)
	}
	return groupByPlacement
}
