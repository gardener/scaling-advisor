package scalein

import (
	"sync"
	"sync/atomic"

	"github.com/gardener/scaling-advisor/planner/pdbtracker"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
)

// SimulatorState holds the internal request-scoped state of a ScaleInSimulator.
type SimulatorState struct {
	// SimulationFactory is used to create `ScaleInSimulation`s
	SimulationFactory plannerapi.SimulationFactory
	// Request is the planner request being currently satisfied.
	Request  *plannerapi.Request
	ResultCh chan plannerapi.ScaleInPlanResult
	// SimRunCounter is a run counter for the number of simulation runs
	SimRunCounter       *atomic.Uint32
	PdbTracker          plannerapi.PDBTracker
	ScaleInNomineeNodes map[string]sacorev1alpha1.ScaleInItem
	Memento             plannerapi.ScaleInMemento
	view                minkapi.View
	simConfig           plannerapi.SimulatorConfig
	mu                  sync.Mutex
}

// NewSimulatorState constructs a fresh SimulatorState with the given planner Request and parameters.
func NewSimulatorState(request *plannerapi.Request, simConfig plannerapi.SimulatorConfig,
	simulationFactory plannerapi.SimulationFactory, requestView minkapi.View) SimulatorState {
	return SimulatorState{
		Request:             request,
		ResultCh:            make(chan plannerapi.ScaleInPlanResult),
		SimulationFactory:   simulationFactory,
		SimRunCounter:       &atomic.Uint32{},
		simConfig:           simConfig,
		PdbTracker:          pdbtracker.New(),
		ScaleInNomineeNodes: make(map[string]sacorev1alpha1.ScaleInItem),
		Memento:             request.Memento.ScaleIn,
		view:                requestView,
	}
}

// SetRequestView sets the pre-initialized request view as the base view for simulations.
func (s *SimulatorState) SetRequestView(view minkapi.View) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.view = view
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
	if s.SimRunCounter != nil {
		s.SimRunCounter.Store(0)
	}
	s.Request = nil
	return nil
}

// SendPlanResult sends a successful ScaleInPlanResult to the given channel.
func SendPlanResult(requestRef plannerapi.RequestRef, planResultCh chan<- plannerapi.ScaleInPlanResult, view minkapi.View, memento plannerapi.ScaleInMemento, scaleInItems []sacorev1alpha1.ScaleInItem) {
	planResult := plannerapi.ScaleInPlanResult{
		Memento: memento,
		//TODO: What labels to send?
		Labels: map[string]string{
			commonconstants.LabelRequestID: requestRef.ID,
		},
		ScaleInPlan: &sacorev1alpha1.ScaleInPlan{
			Items: scaleInItems,
		},
		View: view,
	}

	planResultCh <- planResult
}

// SendPlanError wraps the given error with the sentinel ErrGenScalingPlan and returns it as a ScaleInPlanResult.
func SendPlanError(requestRef plannerapi.RequestRef, planResultCh chan<- plannerapi.ScaleInPlanResult, view minkapi.View, memento plannerapi.ScaleInMemento, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	planResultCh <- plannerapi.ScaleInPlanResult{
		Error:   err,
		Memento: memento,
		View:    view,
	}
}
