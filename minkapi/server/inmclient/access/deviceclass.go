package access

import (
	"context"
	"fmt"

	"github.com/gardener/scaling-advisor/minkapi/server/typeinfo"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/applyconfigurations/resource/v1"
	clientresourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
)

var (
	_ clientresourcev1.DeviceClassInterface = (*deviceClassAccess)(nil)
)

type deviceClassAccess struct {
	BasicResourceAccess[*resourcev1.DeviceClass, *resourcev1.DeviceClassList]
}

func NewDeviceClassAccess(view mkapi.View) clientresourcev1.DeviceClassInterface {
	return &deviceClassAccess{
		BasicResourceAccess[*resourcev1.DeviceClass, *resourcev1.DeviceClassList]{
			view:            view,
			gvk:             typeinfo.DeviceClassDescriptor.GVK,
			Namespace:       metav1.NamespaceNone,
			ResourcePtr:     &resourcev1.DeviceClass{},
			ResourceListPtr: &resourcev1.DeviceClassList{},
		},
	}
}

func (a *deviceClassAccess) Create(ctx context.Context, deviceClass *resourcev1.DeviceClass, opts metav1.CreateOptions) (*resourcev1.DeviceClass, error) {
	return a.createObjectWithAccessNamespace(ctx, opts, deviceClass)
}

func (a *deviceClassAccess) Update(ctx context.Context, deviceClass *resourcev1.DeviceClass, opts metav1.UpdateOptions) (*resourcev1.DeviceClass, error) {
	return a.updateObject(ctx, opts, deviceClass)
}

func (a *deviceClassAccess) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return a.deleteObject(ctx, opts, a.Namespace, name)
}

func (a *deviceClassAccess) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return a.deleteObjectCollection(ctx, a.Namespace, opts, listOpts)
}

func (a *deviceClassAccess) Get(ctx context.Context, name string, opts metav1.GetOptions) (*resourcev1.DeviceClass, error) {
	return a.getObject(ctx, a.Namespace, name, opts)
}

func (a *deviceClassAccess) List(ctx context.Context, opts metav1.ListOptions) (*resourcev1.DeviceClassList, error) {
	return a.getObjectList(ctx, a.Namespace, opts)
}

func (a *deviceClassAccess) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return a.getWatcher(ctx, a.Namespace, opts)
}

func (a *deviceClassAccess) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *resourcev1.DeviceClass, err error) {
	if len(subresources) > 0 {
		return nil, fmt.Errorf("%w: patch of subresources %q is invalid for %q", commonerrors.ErrInvalidOptVal, subresources, a.gvk.Kind)
	}
	return a.patchObject(ctx, name, pt, data, opts)
}

func (a *deviceClassAccess) Apply(ctx context.Context, deviceClass *v1.DeviceClassApplyConfiguration, opts metav1.ApplyOptions) (result *resourcev1.DeviceClass, err error) {
	return nil, fmt.Errorf("%w: apply is not implemented for %q", commonerrors.ErrUnimplemented, a.gvk.Kind)
}
