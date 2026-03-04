package scalein

import (
	"context"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sync/atomic"
)

// RunState holds internal run state details of parent ScaleInSimulation.
type RunState struct {
	err                         error
	ctx                         context.Context
	view                        minkapi.View
	scaleInNodes                map[string]*corev1.Node                                   // map of node names to scale-in nodes
	unscheduledPods             map[commontypes.NamespacedName]plannerapi.PodResourceInfo // map of unscheduled Pod namespacedName to PodResourceInfo
	scheduledPodNamesByNodeName map[string]sets.Set[commontypes.NamespacedName]           // map of node names to a set of scheduled pod names
	leftoverUnscheduledPodNames sets.Set[commontypes.NamespacedName]                      // represents a set of pod names scheduled during simulation run
	status                      plannerapi.ActivityStatus
	name                        string
	traceDir                    string
	numUnchangedTrackAttempts   int
	numTrackAttempts            int
	numReceivedEvents           int
	numScheduledPods            int
	runNum                      uint32
	nodeCount                   atomic.Uint32
}

// FreshRunState returns a fresh RunState whose status is set to [plannerapi.ActivityStatusPending]
func FreshRunState() RunState {
	return RunState{
		status:                      plannerapi.ActivityStatusPending,
		scheduledPodNamesByNodeName: make(map[string]sets.Set[commontypes.NamespacedName]),
		scaleInNodes:                make(map[string]*corev1.Node),
	}
}

// Init initializes this RunState from the given params, changes the [RunState]'s [plannerapi.ActivityStatus] to
// [plannerapi.ActivityStatusRunning] and returns the child run context or an error. The view is also interrogated for
// initializing unscheduledPods. This method must be invoked before calling other
// methods of [RunState]
func (r *RunState) Init(parentCtx context.Context, name string, runNum uint32, view minkapi.View, traceDir string) (context.Context, error) {
	//TODO implement me
	panic("implement me")
}
