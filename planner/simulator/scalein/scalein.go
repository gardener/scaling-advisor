package scalein

import (
	"context"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"sync"
	"sync/atomic"
)

var _ plannerapi.ScaleInSimulator = (*defaultSimulator)(nil)

type defaultSimulator struct {
	viewAccess        minkapi.ViewAccess
	schedulerLauncher plannerapi.SchedulerLauncher
	traceDir          string
	state             RequestState
	simulatorConfig   plannerapi.SimulatorConfig //TODO: Should this be plannerapi.ScaleInSimulatorConfig?
}

func (d *defaultSimulator) Close() error {
	//TODO implement me
	panic("implement me")
}

func (d *defaultSimulator) Simulate(ctx context.Context, requestRef plannerapi.RequestRef, state *plannerapi.ScaleInMemento, requestView minkapi.View, factory plannerapi.SimulationFactory) <-chan plannerapi.ScaleInPlanResult {
	//TODO implement me
	panic("implement me")
}

// New creates a new plannerapi.ScaleInSimulator that runs simulations for scaled in nodes.
func New(args plannerapi.SimulatorArgs) (plannerapi.ScaleInSimulator, error) {
	return &defaultSimulator{
		simulatorConfig:   args.Config,
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
	simConfig     plannerapi.SimulatorConfig
	mu            sync.Mutex
}

// RequestStateWith constructs a fresh simulator RequestState with the given planner Request and parameters
func RequestStateWith(request *plannerapi.Request, simConfig plannerapi.SimulatorConfig,
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
