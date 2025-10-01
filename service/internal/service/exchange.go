package service

import (
	"context"
	"errors"
	svcapi "github.com/gardener/scaling-advisor/api/service"
)

type exchange struct {
	Ctx          context.Context
	CancelFunc   context.CancelFunc
	Request      svcapi.ScalingAdviceRequest
	AdviceEvents []svcapi.ScalingAdviceEvent
	EventChannel <-chan svcapi.ScalingAdviceEvent
}

func (e *exchange) GetError() error {
	var errs []error
	for _, event := range e.AdviceEvents {
		if event.Err != nil {
			errs = append(errs, event.Err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
