// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strconv"

	"github.com/gardener/scaling-advisor/planner/simulator"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/gardener/scaling-advisor/api/planner"
	"github.com/gardener/scaling-advisor/common/objutil"
)

// SendPlanError wraps the given error with request ref info, embeds the wrapped error within a ScalingAdviceResult and sends the same to the given results channel.
func SendPlanError(resultsCh chan<- planner.ScalingPlanResult, requestRef planner.ScalingAdviceRequestRef, err error) {
	err = planner.AsPlanError(requestRef.ID, requestRef.CorrelationID, err)
	resultsCh <- planner.ScalingPlanResult{
		Name: objutil.GenerateName("plan-error"),
		Err:  err,
	}
}

// SendPlanResult creates a ScalingPlanResult from the given SimulationGroupRunResult and sends it to the provided result channel.
func SendPlanResult(req *planner.ScalingAdviceRequest, sgrr planner.SimulationGroupRunResult, resultCh chan<- planner.ScalingPlanResult) error {
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
	resultCh <- planner.ScalingPlanResult{
		Name:         objutil.GenerateName("plan"),
		Labels:       labels,
		ScaleOutPlan: &scaleOutPlan,
	}
	return nil
}
