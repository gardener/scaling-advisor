package access

import (
	"context"
	"fmt"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	corev1 "k8s.io/api/core/v1"
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
	BasicResourceAccess[*corev1.Event, *corev1.EventList]
}

func NewEventAccess(view mkapi.View, namespace string) clientcorev1.EventInterface {
	return &eventAccess{
		BasicResourceAccess[*corev1.Event, *corev1.EventList]{
			view:            view,
			gvk:             typeinfo.EventsDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &corev1.Event{},
			ResourceListPtr: &corev1.EventList{},
		},
	}
}

func (a *eventAccess) Create(ctx context.Context, event *corev1.Event, opts metav1.CreateOptions) (*corev1.Event, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, event)
}
func (a *eventAccess) CreateWithEventNamespaceWithContext(ctx context.Context, event *corev1.Event) (*corev1.Event, error) {
	return a.createObject(ctx, metav1.CreateOptions{}, event)
}
func (a *eventAccess) CreateWithEventNamespace(event *corev1.Event) (*corev1.Event, error) {
	return a.createObject(context.Background(), metav1.CreateOptions{}, event)
}

func (a *eventAccess) Update(ctx context.Context, event *corev1.Event, opts metav1.UpdateOptions) (*corev1.Event, error) {
	return a.updateObject(ctx, opts, event)
}
func (a *eventAccess) UpdateWithEventNamespaceWithContext(ctx context.Context, event *corev1.Event) (*corev1.Event, error) {
	//TODO implement me
	panic("implement me")
}
func (a *eventAccess) UpdateWithEventNamespace(event *corev1.Event) (*corev1.Event, error) {
	//TODO implement me
	panic("implement me")
}

func (a *eventAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *eventAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *eventAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Event, error) {
	return a.getObject(ctx, a.Namespace, name)
}

func (a *eventAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.EventList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *eventAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *eventAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.Event, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for events", commonerrors.ErrInvalidOptVal, subresources)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *eventAccess) PatchWithEventNamespace(event *corev1.Event, data []byte) (*corev1.Event, error) {
	//TODO implement me
	panic("implement me")
}

func (a *eventAccess) PatchWithEventNamespaceWithContext(ctx context.Context, event *corev1.Event, data []byte) (*corev1.Event, error) {
	//TODO implement me
	panic("implement me")
}

func (a *eventAccess) Apply(ctx context.Context, event *v1.EventApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Event, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *eventAccess) Search(scheme *runtime.Scheme, objOrRef runtime.Object) (*corev1.EventList, error) {
	//TODO implement me
	panic("implement me")
}
func (a *eventAccess) SearchWithContext(ctx context.Context, scheme *runtime.Scheme, objOrRef runtime.Object) (*corev1.EventList, error) {
	//TODO implement me
	panic("implement me")
}

func (a *eventAccess) GetFieldSelector(involvedObjectName, involvedObjectNamespace, involvedObjectKind, involvedObjectUID *string) fields.Selector {
	panic(commonerrors.ErrUnimplemented)
}
