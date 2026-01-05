package util

import (
	"strconv"
	"time"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/service/internal/core/simulator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CreateScalingAdviceResponse creates a ScalingAdviceResponse based on the given request and plan result.
func CreateScalingAdviceResponse(request service.ScalingAdviceRequest, planResult service.ScalingPlanResult) *service.ScalingAdviceResponse {
	scalingAdvice := sacorev1alpha1.ClusterScalingAdvice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      planResult.Name,
			Namespace: request.Constraint.Namespace,
			Labels:    planResult.Labels, // TODO use merge when there are additional labels to add other than from planResult
		},
		Spec: sacorev1alpha1.ClusterScalingAdviceSpec{
			ScaleOutPlan: planResult.ScaleOutPlan,
			ScaleInPlan:  planResult.ScaleInPlan,
			ConstraintRef: commontypes.ConstraintReference{
				Name:      request.Constraint.Name,
				Namespace: request.Constraint.Namespace,
			},
		},
	}
	scalingAdviceResponse := &service.ScalingAdviceResponse{
		ScalingAdvice: &scalingAdvice,
		Diagnostics:   planResult.Diagnostics,
	}
	return scalingAdviceResponse
}

// SendPlanError wraps the given error with request ref info, embeds the wrapped error within a ScalingAdviceResult and sends the same to the given results channel.
func SendPlanError(resultsCh chan<- service.ScalingPlanResult, requestRef service.ScalingAdviceRequestRef, err error) {
	err = service.AsPlanError(requestRef.ID, requestRef.CorrelationID, err)
	resultsCh <- service.ScalingPlanResult{
		Name: objutil.GenerateName("plan-error"),
		Err:  err,
	}
}

// SimulationGroupRunResult represents the result of running all passes for a SimulationGroup.
type SimulationGroupRunResult struct {
	// Name is the name of the simulation group.
	Name string
	// NumPasses is the number of passes executed in this group before moving to the next group.
	// A pass is defined as the execution of all simulations in a group.
	NumPasses int
	// TotalSimulations is the total number of simulations executed across all groups until now.
	TotalSimulations int
	// CreatedAt is the time when this group run result was created.
	CreatedAt time.Time
	// NextGroupView is the updated view after executing all passes in this group.
	// The next group if any should use this view as its base view.
	NextGroupView minkapi.View
	// WinnerNodeScores contains the node scores of the winning nodes.
	WinnerNodeScores []service.NodeScore
	// LeftoverUnscheduledPods contains the namespaced names of pods that could not be scheduled.
	LeftoverUnscheduledPods []types.NamespacedName
}

func SendPlanResult(req *service.ScalingAdviceRequest, sgrr SimulationGroupRunResult, resultCh chan<- service.ScalingPlanResult) error {
	existingNodeCountByPlacement, err := req.Snapshot.GetNodeCountByPlacement()
	if err != nil {
		return err
	}
	labels := map[string]string{
		commonconstants.LabelRequestID:                req.ID,
		commonconstants.LabelCorrelationID:            req.CorrelationID,
		commonconstants.LabelSimulationGroupName:      sgrr.Name,
		commonconstants.LabelSimulationGroupNumPasses: strconv.Itoa(sgrr.NumPasses),
		commonconstants.LabelTotalSimulations:         strconv.Itoa(sgrr.TotalSimulations),
	}
	scaleOutPlan := simulator.CreateScaleOutPlan(sgrr.WinnerNodeScores, existingNodeCountByPlacement, sgrr.LeftoverUnscheduledPods)
	resultCh <- service.ScalingPlanResult{
		Name:         objutil.GenerateName("plan"),
		Labels:       labels,
		ScaleOutPlan: &scaleOutPlan,
	}
	return nil
}
