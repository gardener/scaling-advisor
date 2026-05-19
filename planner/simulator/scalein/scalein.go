package scalein

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/minkapi/viewutil"
	"github.com/gardener/scaling-advisor/planner/simulator"
	"github.com/go-logr/logr"
)

// RequestState holds the internal Request scoped state of a ScaleInSimulator
type SimulatorState struct {
	viewAccess minkapi.ViewAccess
	// SimulationFactory is used to create `ScaleInSimulation`s
	SimulationFactory plannerapi.SimulationFactory
	// Request is the planner request being currently satisfied.
	Request  *plannerapi.Request
	ResultCh chan plannerapi.ScaleInPlanResult
	// SimRunCounter is a run counter for the number of simulation runs
	SimRunCounter *atomic.Uint32
	view          minkapi.View
	simConfig     plannerapi.ScaleInSimulatorConfig
	mu            sync.Mutex
}

// RequestStateWith constructs a fresh simulator RequestState with the given planner Request and parameters
func NewSimulatorState(request *plannerapi.Request, simConfig plannerapi.ScaleInSimulatorConfig,
	simulationFactory plannerapi.SimulationFactory, viewAccess minkapi.ViewAccess) SimulatorState {
	return SimulatorState{
		Request:           request,
		ResultCh:          make(chan plannerapi.ScaleInPlanResult),
		SimulationFactory: simulationFactory,
		SimRunCounter:     &atomic.Uint32{},
		simConfig:         simConfig,
		viewAccess:        viewAccess,
	}
}

// InitializeRequestView performs out common initialization on this simulator state.
func (s *SimulatorState) InitializeRequestView(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	requestView, err := s.createRequestView(ctx)
	if err != nil {
		return err
	}

	if err = simulator.PopulateView(ctx, requestView, &s.Request.Snapshot); err != nil {
		err = fmt.Errorf("%w: %w", plannerapi.ErrPopulateRequestView, err)
		return err
	}

	// if r.simConfig.BindVolumeClaimsForImmediateMode {
	// 	// Run static PVC<->PV Binding for Immediate VolumeBinding mode. Can be done just once for in the requestView for all simulations
	// 	if _, err = volutil.BindClaimsForImmediateMode(ctx, requestView); err != nil {
	// 		return err
	// 	}
	// }
	err = viewutil.LogObjects(ctx, "requestView", requestView)
	if err != nil {
		log.Info("failed to dump requestView objects", "requestView", requestView.GetName(), "error", err)
	}
	return nil
}

func (s *SimulatorState) createRequestView(ctx context.Context) (view minkapi.View, err error) {
	view, err = s.viewAccess.GetSandboxViewOverDelegate(ctx, "Request-"+s.Request.ID, s.viewAccess.GetBaseView())
	if err != nil {
		return
	}
	s.view = view
	return
}

// RequestView gets the request minkapi view within this state. request Views are views that only have the request
// cluster snapshot populated within them along with any initialization done by InitializeRequestView.
func (s *SimulatorState) RequestView() minkapi.View {
	return s.view
}

// Reset clears and resets this RequestState
func (s *SimulatorState) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view != nil {
		if err := s.view.Close(); err != nil {
			// best-effort close
			_ = err
		}
	}
	if s.SimRunCounter != nil {
		s.SimRunCounter.Store(0)
	}
	s.Request = nil
	return nil
}

func SendPlanResult(requestRef plannerapi.RequestRef, planResultCh chan<- plannerapi.ScaleInPlanResult, memento plannerapi.ScaleInMemento, scaleInItems []sacorev1alpha1.ScaleInItem) {
	planResult := plannerapi.ScaleInPlanResult{
		Memento: memento,
		Labels: map[string]string{
			commonconstants.LabelRequestID: requestRef.ID,
		},
		ScaleInPlan: &sacorev1alpha1.ScaleInPlan{
			Items: scaleInItems,
		},
	}

	planResultCh <- planResult
}

// SendPlanError wraps the given error with the sentinel ErrGenScalingPlan and returns it as a ScaleInPlanResult.
func SendPlanError(requestRef plannerapi.RequestRef, planResultCh chan<- plannerapi.ScaleInPlanResult, memento plannerapi.ScaleInMemento, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	planResultCh <- plannerapi.ScaleInPlanResult{
		Error:   err,
		Memento: memento,
	}
}
