// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	"github.com/gardener/scaling-advisor/api/minkapi"
	plannerapi "github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/nodeutil"
	"github.com/gardener/scaling-advisor/common/objutil"
	"github.com/gardener/scaling-advisor/common/podutil"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

// SendPlanError wraps the given error with request ref info, embeds the wrapped error within a ScalingAdviceResult and sends the same to the given results channel.
func SendPlanError(resultsCh chan<- plannerapi.ScalingPlanResult, requestRef plannerapi.ScalingAdviceRequestRef, err error) {
	err = plannerapi.AsPlanError(requestRef.ID, requestRef.CorrelationID, err)
	resultsCh <- plannerapi.ScalingPlanResult{
		Name: objutil.GenerateName("plan-error"),
		Err:  err,
	}
}

// SendPlanResult creates a ScalingPlanResult from the given SimulationGroupCycleResults and sends it to the provided result channel.
func SendPlanResult(ctx context.Context, req *plannerapi.ScalingAdviceRequest, resultCh chan<- plannerapi.ScalingPlanResult, simulationRunCount uint32, groupCycleResults []plannerapi.SimulationGroupCycleResult) error {
	log := logr.FromContextOrDiscard(ctx)
	existingNodeCountByPlacement, err := req.Snapshot.GetNodeCountByPlacement()
	if err != nil {
		return err
	}
	labels := map[string]string{
		commonconstants.LabelRequestID:           req.ID,
		commonconstants.LabelCorrelationID:       req.CorrelationID,
		commonconstants.LabelTotalSimulationRuns: fmt.Sprintf("%d", simulationRunCount),
	}
	var allWinnerNodeScores []plannerapi.NodeScore
	var leftOverUnscheduledPods []types.NamespacedName
	for _, gcr := range groupCycleResults {
		allWinnerNodeScores = append(allWinnerNodeScores, gcr.WinnerNodeScores...)
		leftOverUnscheduledPods = gcr.LeftoverUnscheduledPods
	}
	scaleOutPlan := CreateScaleOutPlan(allWinnerNodeScores, existingNodeCountByPlacement, leftOverUnscheduledPods)
	planResult := plannerapi.ScalingPlanResult{
		Name:         objutil.GenerateName("scaling-plan"),
		Labels:       labels,
		ScaleOutPlan: &scaleOutPlan,
	}
	log.Info("Sent ScalingPlanResult", "scalingPlanResult", planResult)
	resultCh <- planResult
	return nil
}

// CreateScaleOutPlan creates a ScaleOutPlan based on the given winningNodeScores, existingNodeCountByPlacement and leftoverUnscheduledPods.
func CreateScaleOutPlan(winningNodeScores []plannerapi.NodeScore, existingNodeCountByPlacement map[sacorev1alpha1.NodePlacement]int32, leftoverUnscheduledPods []types.NamespacedName) sacorev1alpha1.ScaleOutPlan {
	scaleItems := make([]sacorev1alpha1.ScaleOutItem, 0, len(winningNodeScores))
	nodeScoresByPlacement := GroupNodeScoresByNodePlacement(winningNodeScores)
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

// GroupNodeScoresByNodePlacement groups the given nodeScores by their NodePlacement and returns a map of NodePlacement to slice of NodeScores.
func GroupNodeScoresByNodePlacement(nodeScores []plannerapi.NodeScore) map[sacorev1alpha1.NodePlacement][]plannerapi.NodeScore {
	groupByPlacement := make(map[sacorev1alpha1.NodePlacement][]plannerapi.NodeScore)
	for _, ns := range nodeScores {
		groupByPlacement[ns.Placement] = append(groupByPlacement[ns.Placement], ns)
	}
	return groupByPlacement
}

// SynchronizeBaseView synchronizes the given view with the given cluster snapshot.
func SynchronizeBaseView(ctx context.Context, view minkapi.View, cs *plannerapi.ClusterSnapshot) error {
	// TODO implement delta cluster snapshot to update the base view before every simulation run which will synchronize
	// the base view with the current state of the target cluster.
	view.Reset()
	for _, nodeInfo := range cs.Nodes {
		if _, err := view.CreateObject(ctx, typeinfo.NodesDescriptor.GVK, nodeutil.AsNode(nodeInfo)); err != nil {
			return err
		}
	}
	for _, pod := range cs.Pods {
		if _, err := view.CreateObject(ctx, typeinfo.PodsDescriptor.GVK, podutil.AsPod(pod)); err != nil {
			return err
		}
	}
	for _, pc := range cs.PriorityClasses {
		if _, err := view.CreateObject(ctx, typeinfo.PriorityClassesDescriptor.GVK, &pc); err != nil {
			return err
		}
	}
	for _, rc := range cs.RuntimeClasses {
		if _, err := view.CreateObject(ctx, typeinfo.RuntimeClassDescriptor.GVK, &rc); err != nil {
			return err
		}
	}
	// TODO support PVC, PV, StorageClass, etc.
	return nil
}
