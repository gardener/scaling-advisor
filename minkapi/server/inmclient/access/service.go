package access

import (
	"context"
	"fmt"
	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

var (
	_ clientcorev1.ServiceInterface = (*serviceAccess)(nil)
)

type serviceAccess struct {
	BasicResourceAccess[*corev1.Service, *corev1.ServiceList]
}

func NewServiceAccess(view mkapi.View, namespace string) clientcorev1.ServiceInterface {
	return &serviceAccess{
		BasicResourceAccess[*corev1.Service, *corev1.ServiceList]{
			view:            view,
			gvk:             typeinfo.ServicesDescriptor.GVK,
			Namespace:       namespace,
			ResourcePtr:     &corev1.Service{},
			ResourceListPtr: &corev1.ServiceList{},
		},
	}
}

func (a *serviceAccess) Create(ctx context.Context, service *corev1.Service, opts metav1.CreateOptions) (*corev1.Service, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, service)
}

func (a *serviceAccess) Update(ctx context.Context, service *corev1.Service, opts metav1.UpdateOptions) (*corev1.Service, error) {
	return a.updateObject(ctx, opts, service)
}

func (a *serviceAccess) UpdateStatus(ctx context.Context, service *corev1.Service, opts metav1.UpdateOptions) (*corev1.Service, error) {
	return a.updateObject(ctx, opts, service)
}

func (a *serviceAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *serviceAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Service, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *serviceAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.ServiceList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *serviceAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *serviceAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *corev1.Service, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for %q", commonerrors.ErrInvalidOptVal, subresources, a.gvk.Kind)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *serviceAccess) Apply(ctx context.Context, service *v1.ServiceApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Service, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *serviceAccess) ApplyStatus(ctx context.Context, service *v1.ServiceApplyConfiguration, opts metav1.ApplyOptions) (result *corev1.Service, err error) {
	panic(commonerrors.ErrUnimplemented)
}

func (a *serviceAccess) ProxyGet(scheme, name, port, path string, params map[string]string) rest.ResponseWrapper {
	panic(commonerrors.ErrUnimplemented)
}
