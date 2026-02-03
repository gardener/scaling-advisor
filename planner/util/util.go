// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"strconv"
	"time"

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

// SendErrorResponse wraps the given error with the sentinel error plannerapi.ErrGenScalingPlan, embeds the wrapped error
// within a plannerapi.Response and sends the response to the given results channel.
func SendErrorResponse(resultsCh chan<- plannerapi.Response, requestRef plannerapi.RequestRef, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	resultsCh <- plannerapi.Response{
		ID:    objutil.GenerateName("plan-error"),
		Error: err,
	}
}

// SendScaleOutPlanError wraps the given error within a sentinel error plannerapi.ErrGenScalingPlan, creates a ScaleOutPlanResult and
// sends the result on the planResultCh.
func SendScaleOutPlanError(planResultCh chan<- plannerapi.ScaleOutPlanResult, requestRef plannerapi.RequestRef, err error) {
	err = plannerapi.AsGenError(requestRef.ID, requestRef.CorrelationID, err)
	planResultCh <- plannerapi.ScaleOutPlanResult{
		Error: err,
	}
}

// SendScaleOutPlanResult creates a plannerapi.ScaleOutPlanResult from the given plannerapi.Request and plannerapi.SimulationGroupCycleResults
// and sends this result to the resultCh.
func SendScaleOutPlanResult(ctx context.Context, resultCh chan<- plannerapi.ScaleOutPlanResult,
	req *plannerapi.Request, simulationRunCount uint32, // TODO: introduce a plannerapi.Metrics.
	groupCycleResults []plannerapi.SimulationGroupCycleResult) error {
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
	var leftOverUnscheduledPods []types.NamespacedName
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

// createScaleOutPlan creates a ScaleOutPlan based on the given winningNodeScores, existingNodeCountByPlacement and leftoverUnscheduledPods.
func createScaleOutPlan(winningNodeScores []plannerapi.NodeScore, existingNodeCountByPlacement map[sacorev1alpha1.NodePlacement]int32, leftoverUnscheduledPods []types.NamespacedName) sacorev1alpha1.ScaleOutPlan {
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

// SynchronizeView synchronizes the given view with the given cluster snapshot.
func SynchronizeView(ctx context.Context, view minkapi.View, cs *plannerapi.ClusterSnapshot) error {
	if err := view.Reset(); err != nil {
		return err
	}
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
