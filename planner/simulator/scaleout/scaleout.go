// SPDX-FileCopyrightText: Contributors to the Gardener project
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

	plannerapi "github.com/gardener/scaling-advisor/planner/api"
	"github.com/gardener/scaling-advisor/planner/simulator"

	apicommon "github.com/gardener/scaling-advisor/api/common"
	apicomponent "github.com/gardener/scaling-advisor/api/common/component"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/viewutil"
	"github.com/gardener/scaling-advisor/common/volutil"
	minkapi "github.com/gardener/scaling-advisor/minkapi/api"
	simulationapi "github.com/gardener/scaling-advisor/simulation/api"
	"github.com/go-logr/logr"
)

var (
	_ apicomponent.Resettable = (*SimulatorState)(nil)
)

// SimulatorState holds the internal state of a ScaleOutSimulator when executing simulations
type SimulatorState struct {
	viewAccess minkapi.ViewAccess
	// SimulationFactory is used to create `Simulation`s
	SimulationFactory simulationapi.SimulationFactory
	// Request is the planner request being currently satisfied.
	Request  *plannerapi.Request
	ResultCh chan plannerapi.ScaleOutPlanResult
	// SimRunCounter is a run counter for the number of simulation runs
	SimRunCounter *atomic.Uint32
	views         []minkapi.View
	// SimulationGroups is the slice of SimulationGroup (if any) created for satisfying the current request.
	SimulationGroups []simulationapi.SimulationGroup
	simConfig        plannerapi.SimulatorConfig
	mu               sync.Mutex
}

// NewSimulatorState constructs a fresh [SimulatorState] for the [plannerapi.ScaleOutSimulator] processing the given
// [plannerapi.Request] with the given parameters
func NewSimulatorState(request *plannerapi.Request, simConfig plannerapi.SimulatorConfig, simulationFactory simulationapi.SimulationFactory, viewAccess minkapi.ViewAccess) *SimulatorState {
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

	if err = simulator.PopulateView(ctx, requestView, &s.Request.ClusterSnapshot); err != nil {
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

// SetRequestView sets the given view as the request view in this state (first element of s.views).
// This is used when the caller has pre-built and pre-populated the request view.
func (s *SimulatorState) SetRequestView(view minkapi.View) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.views) == 0 {
		s.views = append(s.views, view)
	} else {
		s.views[0] = view
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
	existingNodeCountByPlacement, err := req.ClusterSnapshot.GetNodeCountByPlacement()
	if err != nil {
		return err
	}
	planGenerateDuration := time.Since(req.CreationTime)
	numUnscheduledPods := len(req.ClusterSnapshot.GetUnscheduledPods())
	labels := map[string]string{
		apicommon.LabelRequestID:                  req.ID,
		apicommon.LabelCorrelationID:              req.CorrelationID,
		apicommon.LabelTotalSimulationRuns:        fmt.Sprintf("%d", simulationRunCount),
		apicommon.LabelPlanGenerateDuration:       planGenerateDuration.String(),
		apicommon.LabelSnapshotNumUnscheduledPods: strconv.Itoa(numUnscheduledPods),
		apicommon.LabelConstraintNumPools:         strconv.Itoa(len(req.ScalingConstraint.NodePools)),
	}
	var allWinnerNodeScores []plannerapi.NodeScore
	var leftOverUnscheduledPods []apicommon.NamespacedName
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

// CreateAllNodeTemplates creates and returns a slice of [simulationapi.ScaleOutNodeTemplate] for all possible
// combinations of NodePools, NodeTemplates and availability zones, resolved from [plannerapi.Request.NodeTemplates]
// and [plannerapi.Request.InstanceTypeDefs].
func CreateAllNodeTemplates(req *plannerapi.Request) []simulationapi.ScaleOutNodeTemplate {
	allNodeTemplates := make([]simulationapi.ScaleOutNodeTemplate, 0)
	for _, np := range req.ScalingConstraint.NodePools {
		if len(np.AvailabilityZones) == 0 {
			continue
		}
		for _, ref := range np.NodeTemplateRefs {
			nt, ok := req.NodeTemplates[ref.Name]
			if !ok {
				continue
			}
			itd, ok := req.InstanceTypeDefs[nt.InstanceType]
			if !ok {
				continue
			}
			for _, az := range np.AvailabilityZones {
				allNodeTemplates = append(allNodeTemplates, createNodeTemplate(np, ref, nt, itd, az))
			}
		}
	}
	return allNodeTemplates
}

// GroupScaleOutNodeTemplatesByPriority does just exactly that and returns a map keyed by GroupKey to slice of
// [simulationapi.ScaleOutNodeTemplate]
func GroupScaleOutNodeTemplatesByPriority(templates []simulationapi.ScaleOutNodeTemplate) map[simulationapi.GroupKey][]simulationapi.ScaleOutNodeTemplate {
	templatesByPriority := make(map[simulationapi.GroupKey][]simulationapi.ScaleOutNodeTemplate)
	for _, t := range templates {
		pk := t.GroupKey
		group, ok := templatesByPriority[pk]
		if !ok {
			group = []simulationapi.ScaleOutNodeTemplate{t}
		} else {
			group = append(group, t)
		}
		templatesByPriority[pk] = group
	}
	return templatesByPriority
}

// createNodeTemplate creates a [simulationapi.ScaleOutNodeTemplate] for the given [sacorev1alpha1.NodePool],
// [sacorev1alpha1.NodeTemplateRef], [sacorev1alpha1.NodeTemplate], [sacorev1alpha1.InstanceTypeDef] and availability zone.
func createNodeTemplate(pool sacorev1alpha1.NodePool, ref sacorev1alpha1.NodeTemplateRef, nt sacorev1alpha1.NodeTemplate, itd sacorev1alpha1.InstanceTypeDef, zone string) simulationapi.ScaleOutNodeTemplate {
	return simulationapi.ScaleOutNodeTemplate{
		NodePlacement: sacorev1alpha1.NodePlacement{
			PoolName:         pool.Name,
			TemplateName:     nt.Name,
			InstanceType:     nt.InstanceType,
			Region:           pool.Region,
			AvailabilityZone: zone,
		},
		Labels:            pool.Labels,
		Annotations:       pool.Annotations,
		Quota:             ref.Quota,
		Taints:            pool.Taints,
		BaseResources:     itd.BaseResources,
		ExtendedResources: nt.ExtendedResources,
		ReservedResources: nt.ReservedResources,
		Architecture:      itd.Architecture,
		GroupKey: simulationapi.GroupKey{
			NodePoolPriority:     pool.Priority,
			NodeTemplatePriority: ref.Priority,
		},
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
func createScaleOutPlan(winningNodeScores []plannerapi.NodeScore, existingNodeCountByPlacement map[sacorev1alpha1.NodePlacement]int32, leftoverUnscheduledPods []apicommon.NamespacedName) sacorev1alpha1.ScaleOutPlan {
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
