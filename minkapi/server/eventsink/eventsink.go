// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package eventsink

import (
	"context"
	"fmt"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"slices"

	"github.com/go-logr/logr"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

var _ mkapi.EventSink = (*InMemEventSink)(nil)

type InMemEventSink struct {
	log    logr.Logger
	events []eventsv1.Event
}

func New(log logr.Logger) mkapi.EventSink {
	return &InMemEventSink{
		log:    log,
		events: make([]eventsv1.Event, 0, 100),
	}
}

func (s *InMemEventSink) Create(ctx context.Context, event *eventsv1.Event) (*eventsv1.Event, error) {
	s.events = append(s.events, *event)
	return event, nil
}

func (s *InMemEventSink) Update(ctx context.Context, event *eventsv1.Event) (*eventsv1.Event, error) {
	for i, e := range s.events {
		if e.Name == event.Name && e.Namespace == event.Namespace {
			s.events[i] = *event
			return event, nil
		}
	}
	return nil, apierrors.NewNotFound(eventsv1.Resource("events"), event.Name) //TODO: is it plural events or singluar event ?
}

func (s *InMemEventSink) Patch(ctx context.Context, oldEvent *eventsv1.Event, patchData []byte) (patched *eventsv1.Event, err error) {
	for i, e := range s.events {
		if e.Name == oldEvent.Name && e.Namespace == oldEvent.Namespace {
			originalJSON, err := json.Marshal(e)
			if err != nil {
				//TODO: use apierrors here.
				return nil, fmt.Errorf("failed to marshal original event: %w", err)
			}
			patchedJSON, err := strategicpatch.StrategicMergePatch(originalJSON, patchData, eventsv1.Event{})
			if err != nil {
				return nil, fmt.Errorf("failed to apply strategic merge patch: %w", err)
			}

			var patchedEvent eventsv1.Event
			if err := json.Unmarshal(patchedJSON, &patchedEvent); err != nil {
				return nil, fmt.Errorf("failed to unmarshal patched event: %w", err)
			}
			s.events[i] = patchedEvent
			return &s.events[i], nil
		}
	}
	return nil, apierrors.NewNotFound(eventsv1.Resource("events"), oldEvent.Name) //TODO: is it plural events or singluar event ?
}

func (s *InMemEventSink) Delete(ctx context.Context, event *eventsv1.Event) error {
	s.events = slices.DeleteFunc(s.events, func(event eventsv1.Event) bool {
		return event.Name == event.Name && event.Namespace == event.Namespace
	})
	return nil
}

func (s *InMemEventSink) List() []eventsv1.Event {
	return s.events
}

func (s *InMemEventSink) Reset() {
	s.events = nil
}
