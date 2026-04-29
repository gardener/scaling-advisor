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
type RequestState struct {
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

// InitializeRequestView performs out common initialization on this simulator state.
func (r *RequestState) InitializeRequestView(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	requestView, err := r.createRequestView(ctx)
	if err != nil {
		return err
	}

	if err = simulator.PopulateView(ctx, requestView, &r.Request.Snapshot); err != nil {
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

func (r *RequestState) createRequestView(ctx context.Context) (view minkapi.View, err error) {
	view, err = r.viewAccess.GetSandboxViewOverDelegate(ctx, "Request-"+r.Request.ID, r.viewAccess.GetBaseView())
	if err != nil {
		return
	}
	r.view = view
	return
}

// RequestView gets the request minkapi view within this state. request Views are views that only have the request
// cluster snapshot populated within them along with any initialization done by InitializeRequestView.
func (s *RequestState) RequestView() minkapi.View {
	return s.view
}

// Reset clears and resets this RequestState
func (r *RequestState) Reset() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.view != nil {
		if err := r.view.Close(); err != nil {
			// best-effort close
			_ = err
		}
	}
	if r.SimRunCounter != nil {
		r.SimRunCounter.Store(0)
	}
	r.Request = nil
	return nil
}

func SendPlanResult(requestRef plannerapi.RequestRef, planResultCh chan<- plannerapi.ScaleInPlanResult, memento *plannerapi.ScaleInMemento, scaleInItems []sacorev1alpha1.ScaleInItem) {
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
func SendPlanError(requestRef plannerapi.RequestRef, planResultCh chan<- plannerapi.ScaleInPlanResult, memento *plannerapi.ScaleInMemento, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	planResultCh <- plannerapi.ScaleInPlanResult{
		Error:   err,
		Memento: memento,
	}
}
