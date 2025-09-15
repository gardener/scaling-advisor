package generator

import (
	"fmt"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	svcapi "github.com/gardener/scaling-advisor/api/service"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SendError(adviceEventCh chan<- svcapi.ScalingAdviceEvent, requestRef svcapi.ScalingAdviceRequestRef, err error) {
	err = svcapi.AsGenerateError(requestRef.ID, requestRef.CorrelationID, err)
	adviceEventCh <- svcapi.ScalingAdviceEvent{
		Err: err,
	}
}

func createScalingAdvice(request svcapi.ScalingAdviceRequest, groupRunPassNum uint32, winningNodeScores []svcapi.NodeScore, pendingUnscheduledPods []svcapi.PodResourceInfo) (*sacorev1alpha1.ClusterScalingAdvice, error) {
	nodeCountByPlacement, err := request.Snapshot.GetNodeCountByPlacement()
	if err != nil {
		return nil, err
	}
	scaleOutPlan := createScaleOutPlan(winningNodeScores, nodeCountByPlacement, pendingUnscheduledPods)
	return &sacorev1alpha1.ClusterScalingAdvice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", objutil.GenerateName("advice"), groupRunPassNum),
			Namespace: request.Constraint.Namespace,
			Labels: map[string]string{
				commonconstants.LabelSimulationGroupPassNum: fmt.Sprintf("%d", groupRunPassNum),
				commonconstants.LabelRequestID:              request.ScalingAdviceRequestRef.ID,
				commonconstants.LabelCorrelationID:          request.ScalingAdviceRequestRef.CorrelationID,
			},
		},
		Spec: sacorev1alpha1.ClusterScalingAdviceSpec{
			ConstraintRef: commontypes.ConstraintReference{
				Name:      request.Constraint.Name,
				Namespace: request.Constraint.Namespace,
			},
			ScaleOutPlan: &scaleOutPlan,
		},
	}, nil
}

func createScaleOutPlan(winningNodeScores []svcapi.NodeScore, existingNodeCountByPlacement map[sacorev1alpha1.NodePlacement]int, pendingUnscheduledPods []svcapi.PodResourceInfo) sacorev1alpha1.ScaleOutPlan {
	scaleItems := make([]sacorev1alpha1.ScaleOutItem, 0, len(winningNodeScores))
	nodeScoresByPlacement := groupByNodePlacement(winningNodeScores)
	for placement, nodeScores := range nodeScoresByPlacement {
		delta := int32(len(nodeScores))
		currentReplicas := int32(existingNodeCountByPlacement[placement])
		scaleItems = append(scaleItems, sacorev1alpha1.ScaleOutItem{
			NodePlacement:   placement,
			CurrentReplicas: currentReplicas,
			Delta:           delta,
		})
	}
	return sacorev1alpha1.ScaleOutPlan{
		UnsatisfiedPodNames: podutil.GetObjectNamesFromPodResourceInfos(pendingUnscheduledPods),
		Items:               scaleItems,
	}
}

func groupByNodePlacement(nodeScores []svcapi.NodeScore) map[sacorev1alpha1.NodePlacement][]svcapi.NodeScore {
	groupByPlacement := make(map[sacorev1alpha1.NodePlacement][]svcapi.NodeScore)
	for _, ns := range nodeScores {
		groupByPlacement[ns.Placement] = append(groupByPlacement[ns.Placement], ns)
	}
	return groupByPlacement
}
