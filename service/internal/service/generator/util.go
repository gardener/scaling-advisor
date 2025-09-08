package generator

import (
	"fmt"
	svcapi "github.com/gardener/scaling-advisor/api/service"
)

func SendError(adviceEventCh chan<- svcapi.ScalingAdviceEvent, requestRef svcapi.ScalingAdviceRequestRef, err error) {
	err = svcapi.AsGenerateError(requestRef.ID, requestRef.CorrelationID, fmt.Errorf("%w: no unscheduled pods found", svcapi.ErrNoUnscheduledPods))
	adviceEventCh <- svcapi.ScalingAdviceEvent{
		Err: err,
	}
}
