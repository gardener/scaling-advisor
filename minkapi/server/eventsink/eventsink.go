// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package eventsink

import (
	"context"
	"fmt"

	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/go-logr/logr"
	eventsv1 "k8s.io/api/events/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

var _ mkapi.EventSink = (*InMemEventSink)(nil)

// InMemEventSink is plain implementation of minkapi EventSink that holds events in a backing slice.
type InMemEventSink struct {
	log    logr.Logger
	events []eventsv1.Event
}

// New constructs a minkapi event-sink that sinks events to a backing in-memory slice of events.
func New(log logr.Logger) mkapi.EventSink {
	return &InMemEventSink{
		log:    log,
		events: make([]eventsv1.Event, 0, 100),
	}
}

// Create appends the given event to the backing in-memory event slice.
func (s *InMemEventSink) Create(_ context.Context, event *eventsv1.Event) (*eventsv1.Event, error) {
	s.events = append(s.events, *event)
	return event, nil
}

// Update updates the given event within the backing in-memory slice.
func (s *InMemEventSink) Update(_ context.Context, event *eventsv1.Event) (*eventsv1.Event, error) {
	for i, e := range s.events {
		if e.Name == event.Name && e.Namespace == event.Namespace {
			s.events[i] = *event
			return event, nil
		}
	}
	return nil, apierrors.NewNotFound(eventsv1.Resource("events"), event.Name)
}

// Patch updates the given event within the backing in-memory slice.
func (s *InMemEventSink) Patch(_ context.Context, oldEvent *eventsv1.Event, patchData []byte) (patched *eventsv1.Event, err error) {
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
	return nil, apierrors.NewNotFound(eventsv1.Resource("events"), oldEvent.Name)
}

// List lists all the events in the backing in-memory slice.
func (s *InMemEventSink) List() []eventsv1.Event {
	return s.events
}

// Reset clears the backing in-memory slice.
func (s *InMemEventSink) Reset() {
	s.events = nil
}
