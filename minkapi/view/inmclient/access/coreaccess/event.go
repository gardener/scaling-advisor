// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package coreaccess

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"
	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"github.com/gardener/scaling-advisor/api/minkapi"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	_ clientcorev1.EventInterface = (*eventAccess)(nil)
)

type eventAccess struct {
	access.GenericResourceAccess[*corev1.Event, *corev1.EventList]
}

// NewEventAccess creates a new access facade for managing ReplicaSet resources within a specific namespace using the given minkapi View.
func NewEventAccess(view minkapi.View, namespace string) clientcorev1.EventInterface {
	return &eventAccess{
		access.GenericResourceAccess[*corev1.Event, *corev1.EventList]{
			View:      view,
			GVK:       typeinfo.EventsDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *eventAccess) Create(ctx context.Context, event *corev1.Event, opts metav1.CreateOptions) (*corev1.Event, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, event)
}

func (a *eventAccess) CreateWithEventNamespaceWithContext(ctx context.Context, event *corev1.Event) (*corev1.Event, error) {
	return a.CreateObject(ctx, metav1.CreateOptions{}, event)
}

func (a *eventAccess) CreateWithEventNamespace(event *corev1.Event) (*corev1.Event, error) {
	return a.CreateObject(context.Background(), metav1.CreateOptions{}, event)
}

func (a *eventAccess) Update(ctx context.Context, event *corev1.Event, opts metav1.UpdateOptions) (*corev1.Event, error) {
	return a.UpdateObject(ctx, opts, event)
}

func (a *eventAccess) UpdateWithEventNamespaceWithContext(ctx context.Context, event *corev1.Event) (*corev1.Event, error) {
	if event.Name == "" {
		return nil, fmt.Errorf("event name must be specified")
	}
	if event.Namespace == "" {
		return nil, fmt.Errorf("event namespace must be specified")
	}
	e, err := a.Get(ctx, event.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if event.ResourceVersion != "" && event.ResourceVersion != e.ResourceVersion {
		// TODO: I need to make a generic function for this check.
		return nil, errors.NewConflict(
			corev1.Resource("events"),
			event.Name,
			fmt.Errorf("requested ResourceVersion %s does not match current %s", event.ResourceVersion, e.ResourceVersion),
		)
	}
	updatedEvent := e.DeepCopy()
	updateEventFields(updatedEvent, event)
	return a.UpdateObject(ctx, metav1.UpdateOptions{}, updatedEvent)
}

func (a *eventAccess) UpdateWithEventNamespace(event *corev1.Event) (*corev1.Event, error) {
	return a.UpdateWithEventNamespaceWithContext(context.Background(), event)
}

func (a *eventAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *eventAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.DeleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *eventAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Event, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *eventAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.EventList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *eventAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *eventAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.Event, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *eventAccess) PatchWithEventNamespace(event *corev1.Event, data []byte) (*corev1.Event, error) {
	return a.PatchWithEventNamespaceWithContext(context.Background(), event, data)
}

func (a *eventAccess) PatchWithEventNamespaceWithContext(ctx context.Context, event *corev1.Event, data []byte) (*corev1.Event, error) {
	if event.Name == "" {
		return nil, fmt.Errorf("event name must be specified")
	}
	if event.Namespace == "" {
		return nil, fmt.Errorf("event namespace must be specified")
	}
	return a.PatchObject(ctx, event.Name, types.JSONPatchType, data)
}

func (a *eventAccess) Apply(_ context.Context, _ *v1.EventApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Event, err error) {
	return nil, fmt.Errorf("%w: apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *eventAccess) Search(_ *runtime.Scheme, _ runtime.Object) (*corev1.EventList, error) {
	return nil, fmt.Errorf("%w: search of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *eventAccess) SearchWithContext(_ context.Context, _ *runtime.Scheme, _ runtime.Object) (*corev1.EventList, error) {
	return nil, fmt.Errorf("%w: search of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *eventAccess) GetFieldSelector(involvedObjectName, involvedObjectNamespace, involvedObjectKind, involvedObjectUID *string) fields.Selector {
	return fields.SelectorFromSet(fields.Set{
		"involvedObject.name":      *involvedObjectName,
		"involvedObject.namespace": *involvedObjectNamespace,
		"involvedObject.kind":      *involvedObjectKind,
		"involvedObject.uid": func() string {
			if involvedObjectUID != nil {
				return *involvedObjectUID
			}
			return ""
		}(),
	})
}

// updateEventFields updates the relevant fields of the target event from the source event.
func updateEventFields(target, source *corev1.Event) {
	// Update fields that are typically modified in an event update
	target.Reason = source.Reason
	target.Message = source.Message
	target.Count = source.Count
	target.LastTimestamp = source.LastTimestamp
	target.Type = source.Type
	target.Source = source.Source
	// Preserve fields like InvolvedObject and FirstTimestamp unless explicitly set
	if source.InvolvedObject.Name != "" {
		target.InvolvedObject = source.InvolvedObject
	}
	if !source.FirstTimestamp.IsZero() {
		target.FirstTimestamp = source.FirstTimestamp
	}
}
