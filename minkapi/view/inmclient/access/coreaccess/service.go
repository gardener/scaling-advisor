package coreaccess

import (
	"context"
	"fmt"
	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access"

	"github.com/gardener/scaling-advisor/minkapi/view/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
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
	access.GenericResourceAccess[*corev1.Service, *corev1.ServiceList]
}

// NewServiceAccess creates a new access facade for managing Service resources within a specific namespace using the given minkapi View.
func NewServiceAccess(view mkapi.View, namespace string) clientcorev1.ServiceInterface {
	return &serviceAccess{
		access.GenericResourceAccess[*corev1.Service, *corev1.ServiceList]{
			View:      view,
			GVK:       typeinfo.ServicesDescriptor.GVK,
			Namespace: namespace,
		},
	}
}

func (a *serviceAccess) Create(ctx context.Context, service *corev1.Service, opts metav1.CreateOptions) (*corev1.Service, error) {
	return a.CreateObjectWithAccessNamespace(ctx, opts, service)
}

func (a *serviceAccess) Update(ctx context.Context, service *corev1.Service, opts metav1.UpdateOptions) (*corev1.Service, error) {
	return a.UpdateObject(ctx, opts, service)
}

func (a *serviceAccess) UpdateStatus(ctx context.Context, service *corev1.Service, opts metav1.UpdateOptions) (*corev1.Service, error) {
	return a.UpdateObject(ctx, opts, service)
}

func (a *serviceAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.DeleteObject(ctx, a.Namespace, name, opts)
}

func (a *serviceAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Service, error) {
	return a.GetObject(ctx, a.Namespace, name, opts)
}

func (a *serviceAccess) List(ctx context.Context, opts metav1.ListOptions) (*corev1.ServiceList, error) {
	return a.GetObjectList(ctx, a.Namespace, opts)
}

func (a *serviceAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.GetWatcher(ctx, a.Namespace, opts)
}

func (a *serviceAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subResources ...string) (result *corev1.Service, err error) {
	return a.PatchObject(ctx, name, pt, data, subResources...)
}

func (a *serviceAccess) Apply(_ context.Context, _ *v1.ServiceApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Service, err error) {
	return nil, fmt.Errorf("%w: Apply of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *serviceAccess) ApplyStatus(_ context.Context, _ *v1.ServiceApplyConfiguration, _ metav1.ApplyOptions) (result *corev1.Service, err error) {
	return nil, fmt.Errorf("%w: ApplyStatus of %q is not supported", commonerrors.ErrUnimplemented, a.GVK.Kind)
}

func (a *serviceAccess) ProxyGet(_, _, _, _ string, _ map[string]string) rest.ResponseWrapper {
	panic(commonerrors.ErrUnimplemented)
}
